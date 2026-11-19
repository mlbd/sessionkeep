// Package web is the local dashboard: a loopback-only HTTP server with a small JSON API and
// static pages embedded in the binary.
//
// The API adds no logic of its own. Every endpoint maps to the same sessionkeep command a person
// would type, run in-process through Runner, so consent rules, scope checks and wording are
// identical in the terminal, the MCP server and the browser.
//
// Trust model (docs/07-SECURITY_PRIVACY.md): any local web page the user visits can send
// requests to 127.0.0.1, so being on loopback is not authentication. A request is accepted only
// if it carries this server's session cookie, names this server in its Host header, does not
// come from another origin, and — when it changes anything — carries the session's CSRF token.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed ui
var uiFS embed.FS

// Runner executes one sessionkeep command and returns what it printed and its exit status.
type Runner func(args []string) (stdout, stderr []byte, code int)

// Exit statuses of the command line, mirrored here to avoid importing it.
const (
	exitOK           = 0
	exitUsage        = 2
	exitUnavailable  = 3
	exitNeedsConsent = 4
)

const (
	cookieName     = "sessionkeep_session"
	csrfHeader     = "X-SessionKeep-CSRF"
	bootstrapTTL   = 2 * time.Minute
	sessionIdleTTL = 8 * time.Hour
	maxBody        = 1 << 20
)

// Server is one dashboard instance. It serves exactly one browser session, created by opening
// the one-time bootstrap link printed on the terminal.
type Server struct {
	run      Runner
	version  string
	listener net.Listener
	origin   string // "http://127.0.0.1:PORT"
	hosts    map[string]bool

	mu        sync.Mutex
	bootstrap string
	bootUntil time.Time
	session   string
	csrf      string
	lastSeen  time.Time
	now       func() time.Time
}

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("no system randomness: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Listen creates a server bound to a free port on the IPv4 loopback address. It never binds
// anything else: there is no option for another address.
func Listen(run Runner, version string) (*Server, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	s := &Server{run: run, version: version, listener: l, now: time.Now,
		origin: fmt.Sprintf("http://127.0.0.1:%d", port),
		hosts:  map[string]bool{fmt.Sprintf("127.0.0.1:%d", port): true, fmt.Sprintf("localhost:%d", port): true},
	}
	s.bootstrap, s.bootUntil = token(), s.now().Add(bootstrapTTL)
	return s, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// BootstrapURL is the one-time link that signs the browser in. It works once, for two minutes.
func (s *Server) BootstrapURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.origin + "/auth?t=" + s.bootstrap
}

// Serve handles requests until the listener is closed.
func (s *Server) Serve() error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	err := srv.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// Close stops the server.
func (s *Server) Close() error { return s.listener.Close() }

// Handler returns the complete request handler, including the security checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	static, _ := fs.Sub(uiFS, "ui")
	files := http.FileServer(http.FS(static))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")

		// DNS rebinding: a hostile page can make its own name resolve to 127.0.0.1, but it cannot
		// make the browser send our address in the Host header.
		if !s.hosts[r.Host] {
			fail(w, http.StatusMisdirectedRequest, "wrong_host", "this server only answers to its own loopback address")
			return
		}
		// Cross-site requests: browsers attach Origin to every cross-origin request and to every
		// state-changing request. Anything naming another origin is refused outright.
		if o := r.Header.Get("Origin"); o != "" && o != s.origin {
			fail(w, http.StatusForbidden, "cross_origin", "requests from other origins are not accepted")
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			fail(w, http.StatusForbidden, "cross_origin", "requests from other sites are not accepted")
			return
		}

		switch {
		case r.URL.Path == "/auth":
			s.auth(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			if !s.authorized(r) {
				fail(w, http.StatusUnauthorized, "not_signed_in", "open the link printed by `sessionkeep dashboard` to sign this browser in")
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				s.mu.Lock()
				ok := subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(s.csrf)) == 1 && s.csrf != ""
				s.mu.Unlock()
				if !ok {
					fail(w, http.StatusForbidden, "csrf", "missing or wrong request token")
					return
				}
				if ct := r.Header.Get("Content-Type"); r.ContentLength != 0 && !strings.HasPrefix(ct, "application/json") {
					fail(w, http.StatusUnsupportedMediaType, "json_only", "send JSON")
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			mux.ServeHTTP(w, r)
		default:
			// The pages themselves hold no data; everything they show comes from /api/.
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				fail(w, http.StatusMethodNotAllowed, "method", "not allowed")
				return
			}
			if r.URL.Path != "/" && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/"), ".") {
				r.URL.Path = "/" // client-side routes all load the one page
			}
			files.ServeHTTP(w, r)
		}
	})
}

// auth exchanges the one-time bootstrap token for a session cookie.
func (s *Server) auth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "method", "not allowed")
		return
	}
	s.mu.Lock()
	given := r.URL.Query().Get("t")
	ok := s.bootstrap != "" && s.now().Before(s.bootUntil) && subtle.ConstantTimeCompare([]byte(given), []byte(s.bootstrap)) == 1
	if ok {
		s.bootstrap = "" // single use
		s.session, s.csrf, s.lastSeen = token(), token(), s.now()
	}
	session := s.session
	s.mu.Unlock()
	if !ok {
		fail(w, http.StatusForbidden, "bad_link", "this sign-in link is wrong, already used or expired; run `sessionkeep dashboard` again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	// Redirect so the token does not stay in the address bar or the page's history entry.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) authorized(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.session)) != 1 {
		return false
	}
	if s.now().Sub(s.lastSeen) > sessionIdleTTL {
		s.session, s.csrf = "", ""
		return false
	}
	s.lastSeen = s.now()
	return true
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	enc.Encode(body)
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	write(w, status, map[string]any{"error": apiError{code, msg}})
}

// decode reads a JSON body strictly: unknown fields and trailing data are errors.
func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "too_large", "request body too large")
		} else {
			fail(w, http.StatusBadRequest, "bad_json", "the request body is not the expected JSON")
		}
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		fail(w, http.StatusBadRequest, "bad_json", "unexpected data after the JSON body")
		return false
	}
	return true
}

// command runs a sessionkeep command with --json and relays the result. A command that needs
// confirmation answers 409 with its preview, so the page can show exactly what would happen.
func (s *Server) command(w http.ResponseWriter, args ...string) {
	out, _, code := s.run(append([]string{"--json"}, args...))
	var data, errBody json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var env struct {
			Data  json.RawMessage `json:"data"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(line), &env) != nil {
			continue
		}
		if env.Data != nil {
			data = env.Data
		}
		if env.Error != nil {
			errBody = env.Error
		}
	}
	status := http.StatusOK
	switch code {
	case exitOK:
	case exitUsage:
		status = http.StatusBadRequest
	case exitNeedsConsent:
		status = http.StatusConflict
	case exitUnavailable:
		status = http.StatusServiceUnavailable
	default:
		status = http.StatusUnprocessableEntity
	}
	body := map[string]any{}
	if data != nil {
		body["data"] = data
	}
	if errBody != nil {
		body["error"] = errBody
	} else if code != exitOK {
		body["error"] = apiError{"failed", "the command failed"}
	}
	write(w, status, body)
}
