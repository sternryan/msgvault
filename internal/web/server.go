// Package web provides an HTTP server for the msgvault web UI.
// It serves a React SPA and a JSON REST API wrapping the query.Engine interface.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
)

// Server serves the web UI and REST API.
type Server struct {
	engine         query.Engine
	attachmentsDir string
	deletions      *deletion.Manager
	logger         *slog.Logger
	dev            bool
}

// NewServer creates a new web server.
func NewServer(engine query.Engine, attachmentsDir string, deletions *deletion.Manager, logger *slog.Logger, dev bool) *Server {
	return &Server{
		engine:         engine,
		attachmentsDir: attachmentsDir,
		deletions:      deletions,
		logger:         logger,
		dev:            dev,
	}
}

// Start listens on the given address and serves until the context is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	srv := &http.Server{
		Addr:    addr,
		Handler: s.applyMiddleware(mux),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	h := &handlers{
		engine:         s.engine,
		attachmentsDir: s.attachmentsDir,
		deletions:      s.deletions,
		logger:         s.logger,
	}

	// API routes
	mux.HandleFunc("GET /api/v1/stats", h.getStats)
	mux.HandleFunc("GET /api/v1/accounts", h.listAccounts)
	mux.HandleFunc("GET /api/v1/aggregate", h.aggregate)
	mux.HandleFunc("GET /api/v1/sub-aggregate", h.subAggregate)
	mux.HandleFunc("GET /api/v1/messages", h.listMessages)
	mux.HandleFunc("GET /api/v1/messages/{id}", h.getMessage)
	mux.HandleFunc("GET /api/v1/messages/{id}/thread", h.getThread)
	mux.HandleFunc("GET /api/v1/search", h.search)
	mux.HandleFunc("GET /api/v1/search/count", h.searchCount)
	mux.HandleFunc("GET /api/v1/attachments/{id}/download", h.downloadAttachment)
	mux.HandleFunc("GET /api/v1/attachments/{id}/inline", h.inlineAttachment)
	mux.HandleFunc("POST /api/v1/deletions/stage", h.stageDeletion)
	mux.HandleFunc("POST /api/v1/deletions/confirm", h.confirmDeletion)
	mux.HandleFunc("GET /api/v1/deletions", h.listDeletions)
	mux.HandleFunc("DELETE /api/v1/deletions/{id}", h.cancelDeletion)

	// SPA fallback: serve frontend for non-API routes
	mux.Handle("/", s.spaHandler())
}

// spaHandler returns an http.Handler that serves the embedded SPA.
// Non-file paths (no extension or not found) serve index.html for client-side routing.
func (s *Server) spaHandler() http.Handler {
	distFS, err := fs.Sub(GetDistFS(), "dist")
	if err != nil {
		// Fallback: serve a placeholder
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>msgvault</h1><p>Frontend not built. Run <code>make web-build</code></p></body></html>`)
		})
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if the file exists in the embedded FS
		cleanPath := strings.TrimPrefix(path, "/")
		if f, err := distFS.Open(cleanPath); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found — serve index.html for SPA client-side routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) applyMiddleware(next http.Handler) http.Handler {
	h := next

	// Panic recovery
	h = recoveryMiddleware(s.logger, h)

	// Request logging
	h = loggingMiddleware(s.logger, h)

	// CORS (dev mode only)
	if s.dev {
		h = corsMiddleware(h)
	}

	return h
}

// apiResponse is the standard JSON response envelope.
type apiResponse struct {
	Data  any            `json:"data,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
	Error string         `json:"error,omitempty"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, resp apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Error: msg})
}
