//go:build js && wasm

package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"syscall/js"
	"time"
)

// The page's end: a pair link opened in this page (the fragment) is kept,
// with a key made for this browser, in this origin's storage; from then on
// the page reaches that server — over WebTransport, its certificate pinned,
// each session signed — whenever it is opened.

//go:embed tab.js
var tabScript string

// Where the pairing is kept, in this origin's localStorage.
const (
	storeServer = "chaosrack.pair"
	storeKey    = "chaosrack.tabkey"
)

// kept is the pairing as stored.
type kept struct {
	WT     string   `json:"wt"`
	Certs  []string `json:"certs"`
	Secret string   `json:"secret,omitempty"` // until the server's hello confirms the pairing
	Audio  string   `json:"audio,omitempty"`
	Rate   int      `json:"rate,omitempty"`
}

// Tab is this page's pairing with a server on another machine.
type Tab struct {
	k     kept
	sk    ed25519.PrivateKey
	api   js.Value
	tries int // hellos that failed in a row
}

var tab *Tab

// Boot takes a pair link from the page's address, once, or the pairing
// kept from an earlier visit, and greets the server. Nil when the page is
// paired with nothing, which is every page but one opened by a link.
func Boot() *Tab {
	if tab != nil {
		return tab
	}
	js.Global().Call("eval", tabScript)
	api := js.Global().Get("__crPair")
	if !api.Truthy() {
		return nil
	}
	t := &Tab{api: api}
	if l, ok := ParseFragment(js.Global().Get("location").Get("hash").String()); ok {
		t.k = kept{WT: l.WT, Certs: l.Certs, Secret: l.Secret, Audio: l.Audio}
		api.Call("strip")
		t.save()
	} else if s := api.Call("get", storeServer).String(); s == "" || json.Unmarshal([]byte(s), &t.k) != nil || t.k.WT == "" || len(t.k.Certs) == 0 {
		return nil
	}
	t.sk = t.key()
	tab = t
	t.expose()
	t.hello()
	return t
}

// Current is the page's pairing, nil when there is none.
func Current() *Tab { return tab }

// key is this browser's, made the first time.
func (t *Tab) key() ed25519.PrivateKey {
	if seed, err := hex.DecodeString(t.api.Call("get", storeKey).String()); err == nil && len(seed) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(seed)
	}
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err) // crypto/rand does not fail
	}
	t.api.Call("set", storeKey, hex.EncodeToString(sk.Seed()))
	return sk
}

func (t *Tab) save() {
	// The secret is kept only until the hello confirms the pairing, and is
	// good only for the server run that printed it.
	if b, err := json.Marshal(t.k); err == nil { //nolint:gosec // see above
		t.api.Call("set", storeServer, string(b))
	}
}

// URL is where a session at path is opened, signed for it now; with the pair
// secret until the server has confirmed the pairing.
func (t *Tab) URL(path string) string {
	q := Sign(t.sk, path, time.Now())
	if t.k.Secret != "" {
		q.Set(QPair, t.k.Secret)
	}
	return "https://" + t.k.WT + path + "?" + q.Encode()
}

// CertHashes are the certificates to pin, base64.
func (t *Tab) CertHashes() []string { return append([]string(nil), t.k.Certs...) }

// Audio is "wt" when the server captures audio.
func (t *Tab) Audio() string { return t.k.Audio }

// Rate is the capture's sample rate, 0 when the server has not said.
func (t *Tab) Rate() int { return t.k.Rate }

// Server is the server's WebTransport host:port.
func (t *Tab) Server() string { return t.k.WT }

// Key is this browser's public key, hex, in full.
func (t *Tab) Key() string { return hex.EncodeToString(t.sk.Public().(ed25519.PublicKey)) } //nolint:forcetypeassert // ed25519's own

// expose gives the page's scripts (the rack link's, page.js) what they need:
// a signed address per session and the certificates.
func (t *Tab) expose() {
	o := js.Global().Get("Object").New()
	o.Set("url", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 {
			return ""
		}
		return t.URL(a[0].String())
	}))
	o.Set("hashes", js.FuncOf(func(js.Value, []js.Value) any {
		return t.api.Call("hashes", certsJS(t.k.Certs))
	}))
	o.Set("server", t.k.WT)
	o.Set("key", t.Key())
	o.Set("state", "greeting")
	js.Global().Set("__crRemote", o)
}

func certsJS(certs []string) js.Value {
	a := js.Global().Get("Array").New()
	for _, c := range certs {
		a.Call("push", c)
	}
	return a
}

// helloTries is how many hellos a page makes before it gives up until the next
// load.
const helloTries = 3

// hello opens the first session: it pairs this browser if the secret is
// still held, and brings the certificates and what the server offers up
// to date.
func (t *Tab) hello() {
	var onOK, onErr js.Func
	release := func() { onOK.Release(); onErr.Release() }
	remote := js.Global().Get("__crRemote")
	onOK = js.FuncOf(func(_ js.Value, a []js.Value) any {
		defer release()
		var h Hello
		if len(a) == 0 || json.Unmarshal([]byte(a[0].String()), &h) != nil {
			t.say(remote, "error", "the server's hello was unreadable")
			return nil
		}
		if h.V != Version {
			t.say(remote, "error", "the server speaks pairing v"+strconv.Itoa(h.V)+", this page v"+strconv.Itoa(Version)+": update one of them")
			return nil
		}
		if len(h.Certs) > 0 {
			t.k.Certs = h.Certs
		}
		t.k.Audio, t.k.Rate = h.Audio, h.Rate
		if h.Key == t.Key() {
			t.k.Secret = "" // paired: the secret is the run's, and no longer needed
		}
		t.tries = 0
		t.save()
		t.say(remote, "paired", "paired with "+t.k.WT+" as "+t.Key())
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, a []js.Value) any {
		defer release()
		why := "no answer"
		if len(a) > 0 {
			why = js.Global().Get("String").Invoke(a[0]).String()
		}
		t.say(remote, "error", t.k.WT+" did not answer ("+why+"); if it was restarted with a new certificate, or this browser was never paired, open the pair link chaosrack serve prints")
		// Tried again a few times: a page busy booting, or a server just
		// restarting, answers the next one.
		t.tries++
		if t.tries < helloTries {
			var again js.Func
			again = js.FuncOf(func(js.Value, []js.Value) any {
				again.Release()
				t.hello()
				return nil
			})
			js.Global().Call("setTimeout", again, 10000)
		}
		return nil
	})
	t.api.Call("hello", t.URL(PathHello), certsJS(t.k.Certs)).Call("then", onOK, onErr)
}

// say records the pairing's state where the page can read it, and in the
// console.
func (t *Tab) say(remote js.Value, state, msg string) {
	remote.Set("state", state)
	remote.Set("why", msg)
	method := "log"
	if state == "error" {
		method = "warn"
	}
	js.Global().Get("console").Call(method, "chaosrack pairing: "+msg)
}
