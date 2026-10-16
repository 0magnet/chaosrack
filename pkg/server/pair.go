//go:build !js

package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/webtransport-go"

	"github.com/0magnet/chaosrack/pkg/pairing"
	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/wtaudio"
)

// Who may use the privileged channels, and how a browser on another machine
// comes to be one of them (pkg/pairing has the why).
//
// On loopback — the default — nothing here is switched on but the names:
// a page is this machine's when its Origin is its Host and that Host is a
// loopback name or one given with --host.
//
// Bound elsewhere (--bind), the server also pairs browsers. It prints a
// link to the Pages build carrying its WebTransport address, its
// certificate hashes and a secret; a browser that opens it keeps a key, and
// each session it opens is signed with that key inside TLS pinned to the
// certificate. A page fetched from this server over plain http to another
// machine is only for looking at: what it is made of could have been
// changed on the way, and it gets no channel.

var (
	trustedHosts []string
	pairPage     string

	// pairDir is where pairing keeps its files; empty is the user's config
	// directory. A variable for the tests.
	pairDir string

	pairs *pairState // nil while pairing is off
)

func init() {
	runCmd.Flags().StringSliceVar(&trustedHosts, "host", nil, "another name this server's page is reached by on this machine (a reverse proxy's), trusted as localhost is; repeatable")
	runCmd.Flags().StringVar(&pairPage, "pair-page", "https://chaosrack.magnetosphere.net/", "the https page a pair link opens, for browsers on other machines (--bind)")
}

// browserOK is the rule for a page's request: see pairing.Browser.
func browserOK(r *http.Request) bool { return pairing.Browser(r, trustedHosts) }

// trustedPageHost reports whether a page request came by one of this
// machine's names, and so may be offered the channels.
func trustedPageHost(r *http.Request) bool {
	return pairing.TrustedName(hostOf(r.Host), trustedHosts)
}

// pairingWanted reports whether the server is reachable from elsewhere.
func pairingWanted() bool {
	h := listenHost()
	if strings.EqualFold(h, "localhost") {
		return false
	}
	ip := net.ParseIP(h)
	return ip == nil || !ip.IsLoopback()
}

// The files pairing keeps.
const (
	pairedFile   = "paired-tabs"
	certFile     = "wt-cert.pem"
	nextCertFile = "wt-cert-next.pem"
)

// When the certificate is replaced: a next one is made a week before the
// current one ends, so a paired page that visits in that week learns it,
// and it is served from a day before.
const (
	certNextAt  = 7 * 24 * time.Hour
	certSwapAt  = 24 * time.Hour
	certCheckEv = time.Hour
)

// helloWait is the longest a hello session is held for its page.
const helloWait = 2 * time.Minute

type pairState struct {
	mu     sync.Mutex
	dir    string
	keys   map[string]bool // paired keys, hex
	secret string
	seen   map[string]time.Time // signatures used, until they are too old to be offered
	cur    *wtaudio.Cert
	next   *wtaudio.Cert
	gen    func() (*wtaudio.Cert, error)
}

// preparePairing loads what pairing keeps, when the server is bound to be
// reached from elsewhere. Run before the WebTransport listener is made: its
// certificate is the one kept here.
func preparePairing() error {
	if !pairingWanted() {
		return nil
	}
	dir := pairDir
	if dir == "" {
		cd, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(cd, "chaosrack")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := &pairState{dir: dir, keys: map[string]bool{}, seen: map[string]time.Time{},
		gen: func() (*wtaudio.Cert, error) { return wtaudio.GenerateCert() }}
	if err := p.loadKeys(); err != nil {
		return err
	}
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	p.secret = base64.RawURLEncoding.EncodeToString(b[:])
	p.cur, _ = wtaudio.LoadCert(filepath.Join(dir, certFile))      //nolint:errcheck // none, or unreadable: made again below
	p.next, _ = wtaudio.LoadCert(filepath.Join(dir, nextCertFile)) //nolint:errcheck // likewise
	if _, err := p.rotate(time.Now()); err != nil {
		return err
	}
	pairs = p
	return nil
}

func (p *pairState) loadKeys() error {
	f, err := os.Open(filepath.Join(p.dir, pairedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, _, _ := strings.Cut(strings.TrimSpace(sc.Text()), " "); k != "" && !strings.HasPrefix(k, "#") {
			p.keys[strings.ToLower(k)] = true
		}
	}
	return sc.Err()
}

// rotate brings the certificates up to date at now, keeping them on disk,
// and reports whether the one served changed. Called with p.mu held, or
// before p is shared.
func (p *pairState) rotate(now time.Time) (bool, error) {
	changed := false
	if p.cur == nil || p.cur.NotAfter.Sub(now) < certSwapAt {
		if p.next != nil && p.next.NotAfter.Sub(now) > certSwapAt {
			p.cur, p.next = p.next, nil
		} else {
			c, err := p.gen()
			if err != nil {
				return false, err
			}
			p.cur = c
		}
		changed = true
	}
	if p.next == nil && p.cur.NotAfter.Sub(now) < certNextAt {
		c, err := p.gen()
		if err != nil {
			return changed, err
		}
		p.next = c
	}
	if err := p.cur.Save(filepath.Join(p.dir, certFile)); err != nil {
		return changed, err
	}
	next := filepath.Join(p.dir, nextCertFile)
	if p.next == nil {
		if err := os.Remove(next); err != nil && !errors.Is(err, os.ErrNotExist) {
			return changed, err
		}
		return changed, nil
	}
	return changed, p.next.Save(next)
}

// certs are the hashes a paired page pins: the current certificate's, and
// the next one's once it is made.
func (p *pairState) certs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []string{p.cur.Base64()}
	if p.next != nil {
		out = append(out, p.next.Base64())
	}
	return out
}

// admit decides a session that carries a proof: made for this path, now,
// not seen before, by a paired key — or by a key pairing now, with the
// secret.
func (p *pairState) admit(pr pairing.Proof, path string, now time.Time) error {
	if err := pr.Check(path, now); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for s, at := range p.seen {
		if now.Sub(at) > 2*pairing.Skew {
			delete(p.seen, s)
		}
	}
	sig := string(pr.Sig)
	if _, ok := p.seen[sig]; ok {
		return errors.New("pairing: a proof used before")
	}
	p.seen[sig] = now
	key := pr.KeyHex()
	if p.keys[key] {
		return nil
	}
	if pr.Pair == "" || subtle.ConstantTimeCompare([]byte(pr.Pair), []byte(p.secret)) != 1 {
		return errors.New("pairing: an unpaired browser, " + key)
	}
	f, err := os.OpenFile(filepath.Join(p.dir, pairedFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the server's own directory
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s paired %s\n", key, now.Format(time.RFC3339))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	p.keys[key] = true
	log.Printf("chaosrack: paired a browser: %s", key) //nolint:gosec // hex, made here from the verified key
	return nil
}

// allowWT is the WebTransport listener's rule for every session: the
// terminal's end of the rack link as on the WebSocket; a session with a
// proof by its key; any other as a page of this machine's.
func allowWT(r *http.Request) bool {
	if r.URL.Path == racklink.PathTerminal {
		return linkAllowed(r, true)
	}
	pr, err := pairing.ParseProof(r.URL.Query())
	if errors.Is(err, pairing.ErrNoProof) {
		return browserOK(pagesRequest(r))
	}
	if err == nil {
		if pairs == nil {
			err = errors.New("pairing: this server pairs no browsers (it is bound to loopback)")
		} else {
			err = pairs.admit(pr, r.URL.Path, time.Now())
		}
	}
	if err != nil {
		log.Printf("chaosrack: WebTransport %q from %s refused: %v", r.URL.Path, r.RemoteAddr, err) //nolint:gosec // the path is quoted; the rest is the server's own
		return false
	}
	return true
}

// pagesRequest is a session's CONNECT as the page that opened it would have
// addressed this server: same name, the web port. A session is opened to
// the UDP port, which may be another number (--audio-wt-port).
func pagesRequest(r *http.Request) *http.Request {
	c := r.Clone(r.Context())
	c.Host = net.JoinHostPort(hostOf(r.Host), strconv.Itoa(webPort))
	return c
}

// mountPairing serves the hello and says how to pair, once the
// WebTransport listener is up; nothing at all while pairing is off.
func mountPairing() {
	if pairs == nil {
		return
	}
	if wtSrv == nil {
		log.Printf("chaosrack: browsers on other machines cannot pair: WebTransport is off, and pairing is made over it")
		return
	}
	wtSrv.Handle(pairing.PathHello, serveHello)
	go keepCertsFresh()
	printPairLinks()
}

// serveHello answers a paired page's first session: the certificates to pin
// from now and what this server offers, on a stream of the server's own.
func serveHello(sess *webtransport.Session, r *http.Request) {
	h := pairing.Hello{V: pairing.Version, Certs: pairs.certs(), Rate: audioRate}
	if wtAudio {
		h.Audio = "wt"
	}
	if pr, err := pairing.ParseProof(r.URL.Query()); err == nil {
		h.Key = pr.KeyHex()
	}
	// Held open until the page closes it: a page still booting can take many
	// seconds to read its stream, and closing the session first loses what
	// the stream carries. The cap is for a page that never does.
	ctx, cancel := context.WithTimeout(sess.Context(), helloWait)
	defer cancel()
	str, err := sess.OpenUniStreamSync(ctx)
	if err != nil {
		return
	}
	_ = json.NewEncoder(str).Encode(h) //nolint:errcheck // a page that has gone does not need it
	_ = str.Close()                    //nolint:errcheck // likewise
	<-ctx.Done()                       // the page closes the session when it has read the stream
}

// keepCertsFresh replaces the certificate as it nears its end, for as long
// as the server runs.
func keepCertsFresh() {
	t := time.NewTicker(certCheckEv)
	defer t.Stop()
	for now := range t.C {
		pairs.mu.Lock()
		changed, err := pairs.rotate(now)
		cur := pairs.cur
		pairs.mu.Unlock()
		if err != nil {
			log.Printf("chaosrack: renewing the WebTransport certificate: %v", err)
		}
		if changed {
			wtSrv.SetCert(cur)
			log.Printf("chaosrack: WebTransport certificate now %s (until %s)", cur.Base64(), cur.NotAfter.Format(time.RFC3339))
		}
	}
}

// printPairLinks says how a browser on another machine pairs: one link per
// address this server is reached at.
func printPairLinks() {
	l := pairing.Link{Certs: pairs.certs(), Secret: pairs.secret}
	if wtAudio {
		l.Audio = "wt"
	}
	port := strconv.Itoa(wtPort())
	fmt.Printf("\nchaosrack: reachable from other machines. A browser there pairs once, by opening a link below;\n")
	fmt.Printf("chaosrack:   its page comes from %s over https, and the rack's channels\n", pairPage)
	fmt.Printf("chaosrack:   reach it over WebTransport, encrypted, pinned to this server's certificate.\n")
	fmt.Printf("chaosrack:   The links carry this run's pair secret: open them only in browsers you mean to pair.\n")
	for _, a := range reachableAddrs() {
		l.WT = net.JoinHostPort(a, port)
		fmt.Printf("chaosrack:     %s#%s\n", pairPage, l.Fragment())
	}
	pairs.mu.Lock()
	n := len(pairs.keys)
	pairs.mu.Unlock()
	fmt.Printf("chaosrack:   %d browser(s) paired; %s lists them (delete a line to unpair).\n",
		n, filepath.Join(pairs.dir, pairedFile))
	fmt.Printf("chaosrack:   A page loaded from this server over plain http elsewhere is for looking only.\n\n")
}

// wtPort is the WebTransport listener's port number.
func wtPort() int {
	if audioWTPort != 0 {
		return audioWTPort
	}
	return webPort
}

// reachableAddrs are the addresses another machine reaches this server at:
// the one it is bound to, or each of this machine's when it is bound to all.
func reachableAddrs() []string {
	h := listenHost()
	if ip := net.ParseIP(h); h != "" && (ip == nil || !ip.IsUnspecified()) {
		return []string{h}
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.To4() == nil {
			continue
		}
		out = append(out, n.IP.String())
	}
	sort.Strings(out)
	return out
}

// pageAudioFeed is audioFeed for the page being served: one fetched by a
// name that is not this machine's is given no channel (allowWT, browserOK),
// so it is not told to reach for one.
func pageAudioFeed(r *http.Request) string {
	if !trustedPageHost(r) {
		return ""
	}
	return audioFeed()
}
