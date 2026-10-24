package firmware

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server is a small HTTP server phones download images from. Each update
// job gets a token so the phone's request for "grp2610fw.bin" under that
// token resolves to one exact image. Without a token, "/grp2610fw.bin"
// serves the newest image of that family, which is handy for phones
// pointed at this computer by hand.
type Server struct {
	lib *Library

	mu      sync.Mutex
	srv     *http.Server
	port    int
	tokens  map[string]string // token -> library file
	onFetch func(token, remote string, size int64)
}

func NewServer(lib *Library, onFetch func(token, remote string, size int64)) *Server {
	return &Server{lib: lib, tokens: map[string]string{}, onFetch: onFetch}
}

// Start listens on port (all interfaces). Calling it again on the same port
// is a no-op; a different port restarts the server.
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil && s.port == port {
		return nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("port %d: %w", port, err)
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	s.srv, s.port = srv, port
	go srv.Serve(ln) //nolint:errcheck
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		s.srv.Shutdown(ctx) //nolint:errcheck
		cancel()
		s.srv = nil
	}
}

func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Running reports whether the server is up.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil
}

// Offer makes an image available under a fresh token and returns it.
func (s *Server) Offer(file string) string {
	tok := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36) + strconv.Itoa(len(s.tokens))
	s.mu.Lock()
	s.tokens[tok] = file
	s.mu.Unlock()
	return tok
}

// Withdraw removes a token.
func (s *Server) Withdraw(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := strings.Trim(path.Clean(r.URL.Path), "/")
	parts := strings.Split(p, "/")
	var file, token string
	switch len(parts) {
	case 1: // /grp2610fw.bin -> newest of that family
		fam := FamilyFromFile(parts[0])
		if fam == "" {
			http.NotFound(w, r)
			return
		}
		img, ok := s.lib.Latest(fam)
		if !ok {
			http.NotFound(w, r)
			return
		}
		file = img.File
	case 2: // /<token>/grp2610fw.bin
		s.mu.Lock()
		file, token = s.tokens[parts[0]], parts[0]
		s.mu.Unlock()
		if file == "" {
			http.NotFound(w, r)
			return
		}
		if img, ok := s.lib.Get(file); ok && !strings.EqualFold(parts[1], img.ServedName()) && !strings.EqualFold(parts[1], file) {
			// a phone of another family knocked on this token: don't feed it the wrong image
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	full, err := s.lib.Path(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, full)
	if s.onFetch != nil && r.Method == "GET" {
		img, _ := s.lib.Get(file)
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		s.onFetch(token, host, img.Size)
	}
}

// LocalIP returns the address of this computer that the phone at target
// would reach back to.
func LocalIP(target string) (string, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(target, "9"), 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.IsUnspecified() {
		return "", errors.New("couldn't work out this computer's address")
	}
	return addr.IP.String(), nil
}
