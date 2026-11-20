// Package research fetches a public page the user or their assistant named, turns it into plain
// text, and checks whether a quote is in it. It is the only part of SessionKeep that opens a
// connection to the internet, and it does so only for a URL given in a command.
//
// The page is untrusted: its text is data, never instructions, and it is never rendered as HTML.
package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"sessionkeep/internal/redact"
)

const (
	MaxBody          = 2 << 20 // bytes of a page that are read; the rest is cut and reported
	MaxRedirects     = 5
	MaxURL           = 2000
	Timeout          = 25 * time.Second
	ExtractorVersion = 1
)

// Error is a fetch that did not produce a page. Code is stable: bad_url, blocked_address,
// redirects, http_status, unsupported_type, timeout, unreachable.
type Error struct {
	Code   string
	Status int // for http_status
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// Options adjust one fetch.
type Options struct {
	// AllowLocal lists "host:port" addresses the owner intentionally allowed although they are
	// on this computer or a private network. They may also be reached without TLS.
	AllowLocal []string
	UserAgent  string
	// ETag and LastModified, from an earlier snapshot, make the request conditional.
	ETag, LastModified string
	Timeout            time.Duration
}

// Page is a fetched page as text.
type Page struct {
	URL          string
	FinalURL     string
	Status       int
	NotModified  bool // the server answered 304 to a conditional request; Text is empty
	ETag         string
	LastModified string
	Hash         string // SHA-256 of the bytes read
	Bytes        int
	Truncated    bool
	Title        string
	Text         string
	FetchedAt    time.Time
}

func allowed(hostport string, allow []string) bool {
	for _, a := range allow {
		if strings.EqualFold(a, hostport) {
			return true
		}
	}
	return false
}

func hostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// NormalizeURL checks a URL before anything is sent and returns the form that is stored:
// https only (plain http only for an allowed local address), no user name or password, nothing
// that looks like a secret, no fragment.
func NormalizeURL(raw string, allowLocal []string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxURL || strings.ContainsAny(raw, " \t\r\n\x00") {
		return "", fail("bad_url", "the URL is empty, longer than %d characters or contains spaces", MaxURL)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
		return "", fail("bad_url", "%q is not a full web address (it should start with https://)", raw)
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	if u.User != nil {
		return "", fail("bad_url", "the URL contains a user name or password; SessionKeep does not send or store those")
	}
	local := allowed(hostPort(u), allowLocal)
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", fail("bad_url", "only https:// addresses are fetched (got %q)", u.Scheme)
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !local && blocked(ip) {
		return "", fail("blocked_address", "%s is an address on this computer or a private network; SessionKeep only fetches public pages", u.Hostname())
	}
	u.Fragment, u.RawFragment = "", ""
	out := u.String()
	if _, n := redact.Text(out); n > 0 {
		return "", fail("bad_url", "the URL looks like it contains a password, key or token, so it was not fetched")
	}
	return out, nil
}

var reserved = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2001:db8::/32", "2002::/16", "2001::/32", "100::/64",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// blocked reports whether ip is anything other than an ordinary public address.
func blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Zone() != "" {
		return true
	}
	for _, p := range reserved {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// guard refuses a connection to a non-public address. It runs for every connection, after the
// name was resolved, so a name that resolves to a private address and a redirect to one are
// both refused at the moment of connecting.
func guard(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fail("blocked_address", "cannot read the address %q", address)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || blocked(ip) {
		return fail("blocked_address", "%s is an address on this computer or a private network; SessionKeep only fetches public pages", host)
	}
	return nil
}

func client(o Options) *http.Client {
	strict := &net.Dialer{Timeout: 10 * time.Second, Control: guard}
	plain := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy: nil, // a proxy would hide the real address from the guard
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if allowed(addr, o.AllowLocal) {
				return plain.DialContext(ctx, network, addr)
			}
			return strict.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		DisableKeepAlives:      true,
	}
	return &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > MaxRedirects {
			return fail("redirects", "the page redirected more than %d times", MaxRedirects)
		}
		if _, err := NormalizeURL(req.URL.String(), o.AllowLocal); err != nil {
			var fe *Error
			errors.As(err, &fe)
			return fail(fe.Code, "the page redirects to an address that is not fetched: %s", fe.Msg)
		}
		return nil
	}}
}

func textual(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return contentType == "" // no header at all: decide from the bytes
	}
	return strings.HasPrefix(mt, "text/") || mt == "application/json" || mt == "application/xml" || mt == "application/xhtml+xml" ||
		strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml")
}

// Fetch gets one page. It sends nothing but the URL, a generic User-Agent and, for a re-check,
// the validators of the earlier snapshot. No cookies, no credentials, no proxy.
func Fetch(ctx context.Context, raw string, o Options) (Page, error) {
	clean, err := NormalizeURL(raw, o.AllowLocal)
	if err != nil {
		return Page{}, err
	}
	if o.Timeout <= 0 {
		o.Timeout = Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clean, nil)
	if err != nil {
		return Page{}, fail("bad_url", "%v", err)
	}
	ua := o.UserAgent
	if ua == "" {
		ua = "sessionkeep"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html, text/plain, text/markdown, application/json;q=0.9, */*;q=0.1")
	if o.ETag != "" {
		req.Header.Set("If-None-Match", o.ETag)
	} else if o.LastModified != "" {
		req.Header.Set("If-Modified-Since", o.LastModified)
	}
	p := Page{URL: clean, FetchedAt: time.Now().UTC()}
	resp, err := client(o).Do(req)
	if err != nil {
		var fe *Error
		if errors.As(err, &fe) {
			return Page{}, fe
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return Page{}, fail("timeout", "no answer from %s within %s", req.URL.Hostname(), o.Timeout)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Page{}, fail("timeout", "no answer from %s in time", req.URL.Hostname())
		}
		return Page{}, fail("unreachable", "could not reach %s: %v", req.URL.Hostname(), rootCause(err))
	}
	defer resp.Body.Close()
	p.FinalURL, p.Status = resp.Request.URL.String(), resp.StatusCode
	p.ETag, p.LastModified = clip(resp.Header.Get("ETag"), 300), clip(resp.Header.Get("Last-Modified"), 100)
	if resp.StatusCode == http.StatusNotModified && (o.ETag != "" || o.LastModified != "") {
		p.NotModified = true
		return p, nil
	}
	if resp.StatusCode != http.StatusOK {
		e := fail("http_status", "%s answered %d %s", req.URL.Hostname(), resp.StatusCode, http.StatusText(resp.StatusCode))
		e.Status = resp.StatusCode
		return Page{}, e
	}
	if ct := resp.Header.Get("Content-Type"); !textual(ct) {
		return Page{}, fail("unsupported_type", "the address returns %q, which is not text SessionKeep can read", clip(ct, 80))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil && len(body) == 0 {
		if errors.Is(err, context.DeadlineExceeded) {
			return Page{}, fail("timeout", "the page did not arrive within %s", o.Timeout)
		}
		return Page{}, fail("unreachable", "the connection broke while reading the page: %v", rootCause(err))
	}
	if err != nil {
		p.Truncated = true // a page that broke off part-way is used as far as it came, and says so
	}
	if len(body) > MaxBody {
		body, p.Truncated = body[:MaxBody], true
	}
	sum := sha256.Sum256(body)
	p.Hash, p.Bytes = hex.EncodeToString(sum[:]), len(body)
	p.Title, p.Text = Extract(resp.Header.Get("Content-Type"), string(body))
	return p, nil
}

func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}

func clip(s string, n int) string {
	if len(s) > n {
		return ""
	}
	return s
}
