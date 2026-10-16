package pairing

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Browser reports whether a request may use a privileged channel by being a
// page on this machine (see the package comment), hosts being the names
// the operator added for a reverse proxy.
//
// A request with no Origin is not a browser's: browsers send one on every
// WebSocket, WebTransport and POST. It is a program — the panel, curl — and
// is let in only from this machine.
func Browser(r *http.Request, hosts []string) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return Loopback(r.RemoteAddr)
	}
	u, err := url.Parse(o)
	if err != nil || !strings.EqualFold(u.Host, r.Host) {
		return false
	}
	return TrustedName(u.Hostname(), hosts)
}

// TrustedName reports whether a page at host is one of this machine's: a
// loopback name, or one of hosts. *.localhost is not, though browsers send
// it to loopback: other programs (netscrape, a visor's desk) serve pages
// that are not this server's at names under it.
func TrustedName(host string, hosts []string) bool {
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, h := range hosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	return false
}

// Loopback reports whether addr (host:port, or a bare host) is this machine.
func Loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
