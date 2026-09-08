package dataplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is the production authenticated loopback WebSocket transport. The
// listener binds 127.0.0.1 only, requires the exact Host, validates Origin
// against an explicit allowlist, and authenticates both channels with one-use
// route tokens carried as WebSocket subprotocols.
type Server struct {
	controller      *RouteController
	listenAddr      string
	allowedOrigins  map[string]struct{}
	dataSubprotocol string

	mu            sync.Mutex
	http          *http.Server
	listener      net.Listener
	outputs       map[string]*outputQueue
	urgentHandler UrgentHandler
}

// ServerOption configures the transport.
type ServerOption func(*Server)

// WithOrigins sets the explicit Origin allowlist (empty means deny all).
func WithOrigins(origins ...string) ServerOption {
	return func(s *Server) {
		for _, origin := range origins {
			s.allowedOrigins[origin] = struct{}{}
		}
	}
}

// NewServer builds the transport. The controller owns tokens/generations.
func NewServer(controller *RouteController, listenAddr string, options ...ServerOption) *Server {
	server := &Server{
		controller:      controller,
		listenAddr:      listenAddr,
		allowedOrigins:  map[string]struct{}{},
		dataSubprotocol: "netcatty-terminal-v1",
		outputs:         make(map[string]*outputQueue),
	}
	for _, option := range options {
		option(server)
	}
	return server
}

// Start binds the loopback listener and serves until Stop.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.listenAddr = listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/data/", s.handleData)
	mux.HandleFunc("/v1/urgent/", s.handleUrgent)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.mu.Unlock()
	go func() {
		_ = s.http.Serve(listener)
	}()
	return nil
}

// Addr reports the bound loopback address (host:port).
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop shuts the listener and active connections down.
func (s *Server) Stop() error {
	s.mu.Lock()
	httpServer := s.http
	s.mu.Unlock()
	if httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

var (
	errForbiddenHost   = errors.New("invalid Host")
	errForbiddenOrigin = errors.New("invalid Origin")
	errUnauthorized    = errors.New("route token unauthorized")
)

func (s *Server) authorize(writer http.ResponseWriter, request *http.Request, sessionID string, urgent bool) bool {
	if request.Host != s.listenAddr {
		http.Error(writer, errForbiddenHost.Error(), http.StatusForbidden)
		return false
	}
	origin := request.Header.Get("Origin")
	if _, ok := s.allowedOrigins[origin]; !ok {
		http.Error(writer, errForbiddenOrigin.Error(), http.StatusForbidden)
		return false
	}
	token, ok := routeToken(request.Header.Values("Sec-WebSocket-Protocol"), s.dataSubprotocol)
	if !ok || len(token) != 64 {
		http.Error(writer, errUnauthorized.Error(), http.StatusUnauthorized)
		return false
	}
	// Rebind: generation rides the query string so stale generations fail
	// against the controller's current state.
	generation := uint32(0)
	if _, err := fmt.Sscanf(request.URL.Query().Get("generation"), "%d", &generation); err != nil {
		http.Error(writer, "invalid generation", http.StatusBadRequest)
		return false
	}
	if err := s.controller.Authenticate(sessionID, generation, token, urgent); err != nil {
		http.Error(writer, err.Error(), http.StatusUnauthorized)
		return false
	}
	return true
}

func routeToken(protocols []string, base string) (string, bool) {
	// The client may send one comma-separated header line or several headers.
	var names []string
	for _, line := range protocols {
		for _, part := range strings.Split(line, ",") {
			names = append(names, strings.TrimSpace(part))
		}
	}
	if len(names) != 2 || names[0] != base || !strings.HasPrefix(names[1], "route.") {
		return "", false
	}
	return strings.TrimPrefix(names[1], "route."), true
}
