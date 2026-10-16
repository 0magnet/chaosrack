// Package pairing decides who may use a chaosrack server's privileged
// channels — the audio capture, the rack link, the routing switch — and
// carries the proof that lets a browser on another machine use them.
//
// TWO KINDS OF CALLER, two rules.
//
// A page on this machine is let in by its names (Browser): the request's
// Origin must be its own Host, and that Host must be a loopback name
// (localhost, 127.0.0.0/8, ::1) or one the operator named (--host, for a
// reverse proxy). Checking only that Origin matched Host was not enough: a
// hostile site whose DNS answer is changed to 127.0.0.1 after its page has
// loaded (DNS rebinding) sends a matching pair, evil.example and
// evil.example, from inside the browser. It cannot send Host: localhost —
// the browser writes Host from the address it was given — so naming the
// hosts closes it, with nothing to set up for the ordinary local case.
//
// A browser on another machine is let in by a key (Proof): a page served
// over https (the Pages build) holds an ed25519 key in its own origin's
// storage, and opens each WebTransport session with a signature made by it
// in the CONNECT request's query. The session is QUIC, so TLS 1.3, and the
// page pins the server's certificate by hash, so the query is encrypted to
// a key only the server holds and nothing on the path can read or change
// it. The server keeps the paired PUBLIC keys; a copy of that list grants
// nothing.
//
// A key is paired once, with a pair link the server prints (Link): the
// server's WebTransport address, its certificate hashes and a secret, in
// the link's fragment, which a browser never sends anywhere. It travels
// from the terminal to the browser by hand, so it is the one thing a
// network attacker cannot have.
package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the pairing protocol's. A page and a server that disagree say
// so rather than failing in some stranger way: the Pages build follows main,
// and a server may be older.
const Version = 1

// PathHello is the session a paired page opens first: the server answers
// with a Hello on a stream of its own.
const PathHello = "/pair/hello"

// Skew is how far a proof's time may be from the server's clock.
const Skew = 2 * time.Minute

// Query parameters a proof travels in.
const (
	QKey  = "k"    // the tab's public key, hex
	QTime = "t"    // unix seconds when it was signed
	QSig  = "s"    // the signature, unpadded base64url
	QOnce = "n"    // random, so two proofs made in one second differ: a signature is used once
	QPair = "pair" // the pair secret, on the sessions before a pairing is confirmed
)

// message is what a tab signs for a session at path: the protocol, the
// path, the time and a nonce, so a proof opens only the session it was made
// for, only for a little while, and (the server remembering signatures) once.
func message(path string, t int64, nonce string) []byte {
	return []byte("chaosrack-tab-v" + strconv.Itoa(Version) + "\n" + path + "\n" + strconv.FormatInt(t, 10) + "\n" + nonce)
}

// Sign is the query that proves sk's key for a session at path, made at t.
func Sign(sk ed25519.PrivateKey, path string, t time.Time) url.Values {
	pk := sk.Public().(ed25519.PublicKey) //nolint:forcetypeassert // ed25519's own type
	ts := t.Unix()
	var n [12]byte
	_, _ = rand.Read(n[:]) //nolint:errcheck // crypto/rand does not fail
	nonce := base64.RawURLEncoding.EncodeToString(n[:])
	q := url.Values{}
	q.Set(QOnce, nonce)
	q.Set(QKey, hex.EncodeToString(pk))
	q.Set(QTime, strconv.FormatInt(ts, 10))
	q.Set(QSig, base64.RawURLEncoding.EncodeToString(ed25519.Sign(sk, message(path, ts, nonce))))
	return q
}

// Proof is a session's claim to a key.
type Proof struct {
	Key  ed25519.PublicKey
	Time int64
	Sig  []byte
	Once string
	Pair string // the pair secret, when the tab is pairing
}

// KeyHex is the proof's key as the server lists it: in full, hex.
func (p Proof) KeyHex() string { return hex.EncodeToString(p.Key) }

// ErrNoProof is a request that carries none: a page on this machine, say.
var ErrNoProof = errors.New("pairing: no proof")

// ParseProof reads a proof from a session's query.
func ParseProof(q url.Values) (Proof, error) {
	var p Proof
	if q.Get(QKey) == "" {
		return p, ErrNoProof
	}
	k, err := hex.DecodeString(q.Get(QKey))
	if err != nil || len(k) != ed25519.PublicKeySize {
		return p, errors.New("pairing: an unreadable key")
	}
	t, err := strconv.ParseInt(q.Get(QTime), 10, 64)
	if err != nil {
		return p, errors.New("pairing: an unreadable time")
	}
	s, err := base64.RawURLEncoding.DecodeString(q.Get(QSig))
	if err != nil || len(s) != ed25519.SignatureSize {
		return p, errors.New("pairing: an unreadable signature")
	}
	return Proof{Key: k, Time: t, Sig: s, Once: q.Get(QOnce), Pair: q.Get(QPair)}, nil
}

// Check reports whether the proof was made for path, by its key, near now.
// Whether the key is one the server knows is the server's to say.
func (p Proof) Check(path string, now time.Time) error {
	if d := now.Sub(time.Unix(p.Time, 0)); d > Skew || d < -Skew {
		return errors.New("pairing: the proof's time is " + d.Round(time.Second).String() + " from this machine's clock")
	}
	if !ed25519.Verify(p.Key, message(path, p.Time, p.Once), p.Sig) {
		return errors.New("pairing: the signature does not hold")
	}
	return nil
}

// Link is what a pair link carries, in its fragment.
type Link struct {
	WT     string   // the server's WebTransport host:port
	Certs  []string // the certificate hashes to pin, base64 (wtaudio.Cert.Base64)
	Secret string   // the pair secret
	Audio  string   // "wt" when the server captures audio
}

// Fragment is the link's part after the #.
func (l Link) Fragment() string {
	q := url.Values{}
	q.Set("pair", l.Secret)
	q.Set("wt", l.WT)
	q.Set("cert", strings.Join(l.Certs, ","))
	if l.Audio != "" {
		q.Set("audio", l.Audio)
	}
	return q.Encode()
}

// ParseFragment reads a link from a page's fragment (with or without the #).
// ok is false for a fragment that is not a pair link.
func ParseFragment(frag string) (l Link, ok bool) {
	q, err := url.ParseQuery(strings.TrimPrefix(frag, "#"))
	if err != nil || q.Get("pair") == "" || q.Get("wt") == "" || q.Get("cert") == "" {
		return l, false
	}
	l = Link{WT: q.Get("wt"), Secret: q.Get("pair"), Audio: q.Get("audio")}
	for _, c := range strings.Split(q.Get("cert"), ",") {
		if c != "" {
			l.Certs = append(l.Certs, c)
		}
	}
	return l, len(l.Certs) > 0
}

// Hello is the server's answer on PathHello: what a paired page needs that
// can change between visits.
type Hello struct {
	V     int      `json:"v"`
	Certs []string `json:"certs"`           // the hashes to pin from now: the current certificate, and the next once it is made
	Audio string   `json:"audio,omitempty"` // "wt" when the server captures audio
	Rate  int      `json:"rate,omitempty"`  // the capture's sample rate
	Key   string   `json:"key"`             // the tab's key, as the server paired it
}
