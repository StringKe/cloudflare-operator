// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package mockserver provides a mock Cloudflare API server for testing.
package mockserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/injection"
	"github.com/StringKe/cloudflare-operator/test/mockserver/internal/store"
)

// Server is the mock Cloudflare API server.
type Server struct {
	httpServer    *http.Server
	store         *store.Store
	errorInjector *injection.ErrorInjector
	requestLog    []RequestLogEntry
	requestLogMu  sync.RWMutex
	port          int
}

// RequestLogEntry records an API request.
type RequestLogEntry struct {
	Timestamp time.Time
	Method    string
	Path      string
	Body      string
}

// Option is a function that configures the server.
type Option func(*Server)

// WithPort sets the server port.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// WithStore sets a custom store.
func WithStore(st *store.Store) Option {
	return func(s *Server) {
		s.store = st
	}
}

// NewServer creates a new mock server.
func NewServer(opts ...Option) *Server {
	s := &Server{
		store:         store.NewStore(),
		errorInjector: injection.NewErrorInjector(),
		requestLog:    make([]RequestLogEntry, 0),
		port:          8787,
	}

	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.middleware(mux),
	}

	return s
}

// middleware adds common middleware to all requests.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log request
		s.logRequest(r)

		// Check for error injection
		if err := s.errorInjector.Check(r.URL.Path, r.Method); err != nil {
			s.handleInjectedError(w, err)
			return
		}

		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Auth-Email, X-Auth-Key")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// logRequest logs an incoming request.
func (s *Server) logRequest(r *http.Request) {
	log.Printf("[%s] %s %s", r.Method, r.URL.Path, r.URL.RawQuery)
	s.requestLogMu.Lock()
	defer s.requestLogMu.Unlock()
	s.requestLog = append(s.requestLog, RequestLogEntry{
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
	})
}

// handleInjectedError handles errors from the error injector.
func (s *Server) handleInjectedError(w http.ResponseWriter, err *injection.InjectedError) {
	switch err.Type {
	case injection.ErrorTypeRateLimit:
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		writeError(w, 10000, "Rate limit exceeded")
	case injection.ErrorTypeServerError:
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, 10001, "Internal server error")
	case injection.ErrorTypeTimeout:
		time.Sleep(30 * time.Second) // Simulate timeout
		w.WriteHeader(http.StatusGatewayTimeout)
		writeError(w, 10002, "Request timeout")
	case injection.ErrorTypeConflict:
		w.WriteHeader(http.StatusConflict)
		writeError(w, 10003, "Resource conflict")
	case injection.ErrorTypeNotFound:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 10004, "Resource not found")
	default:
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, 10099, "Unknown error")
	}
}

// Start starts the server.
func (s *Server) Start() error {
	log.Printf("Starting mock Cloudflare API server on port %d", s.port)
	return s.httpServer.ListenAndServe()
}

// StartAsync starts the server in a goroutine and waits for it to be ready.
func (s *Server) StartAsync() error {
	errChan := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for server to be ready
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", s.port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("Mock server ready on port %d", s.port)
				return nil
			}
		}
	}

	select {
	case err := <-errChan:
		return err
	default:
		return fmt.Errorf("server failed to start within timeout")
	}
}

// Stop gracefully stops the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Store returns the server's data store.
func (s *Server) Store() *store.Store {
	return s.store
}

// ErrorInjector returns the server's error injector.
func (s *Server) ErrorInjector() *injection.ErrorInjector {
	return s.errorInjector
}

// Reset resets the server state.
func (s *Server) Reset() {
	s.store.Reset()
	s.errorInjector.Clear()
	s.requestLogMu.Lock()
	s.requestLog = make([]RequestLogEntry, 0)
	s.requestLogMu.Unlock()
}

// GetRequestLog returns a copy of the request log.
func (s *Server) GetRequestLog() []RequestLogEntry {
	s.requestLogMu.RLock()
	defer s.requestLogMu.RUnlock()
	log := make([]RequestLogEntry, len(s.requestLog))
	copy(log, s.requestLog)
	return log
}

// GetRequestCount returns the number of requests matching the given path prefix.
func (s *Server) GetRequestCount(pathPrefix string) int {
	s.requestLogMu.RLock()
	defer s.requestLogMu.RUnlock()
	count := 0
	for _, entry := range s.requestLog {
		if len(entry.Path) >= len(pathPrefix) && entry.Path[:len(pathPrefix)] == pathPrefix {
			count++
		}
	}
	return count
}

// URL returns the base URL of the server.
func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// generateUUID generates a random UUID-like string.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}

// generateToken generates a random token string.
func generateToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// writeSuccess writes a successful response.
func writeSuccess[T any](w http.ResponseWriter, result T) {
	resp := struct {
		Success bool `json:"success"`
		Result  T    `json:"result"`
	}{
		Success: true,
		Result:  result,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, code int, message string) {
	resp := struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}{
		Success: false,
		Errors: []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{{Code: code, Message: message}},
	}
	_ = json.NewEncoder(w).Encode(resp)
}
