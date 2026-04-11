// Package web provides an HTTP server for the msgvault web UI.
// It serves server-rendered HTML using Templ templates and HTMX for dynamic behavior.
package web

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
)

// Server serves the web UI with server-rendered HTML.
type Server struct {
	engine         query.Engine
	attachmentsDir string
	deletions      *deletion.Manager
	logger         *slog.Logger
	aiClient       *ai.Client   // nil if Azure not configured
	store          *store.Store // for vector search; nil if not available
}

// ServerOption configures optional features on Server.
type ServerOption func(*Server)

// WithAI configures the server with an Azure OpenAI client and store for
// semantic and hybrid search. Both must be non-nil.
func WithAI(client *ai.Client, s *store.Store) ServerOption {
	return func(srv *Server) {
		srv.aiClient = client
		srv.store = s
	}
}

// NewServer creates a new web server. Optional features (e.g. AI search)
// can be enabled via ServerOption arguments.
func NewServer(engine query.Engine, attachmentsDir string, deletions *deletion.Manager, logger *slog.Logger, opts ...ServerOption) *Server {
	srv := &Server{
		engine:         engine,
		attachmentsDir: attachmentsDir,
		deletions:      deletions,
		logger:         logger,
	}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// buildRouter constructs and returns the chi router with all routes registered.
// Extracted so tests can reuse it without starting an HTTP listener.
func (s *Server) buildRouter() chi.Router {
	h := &handlers{
		engine:         s.engine,
		attachmentsDir: s.attachmentsDir,
		deletions:      s.deletions,
		logger:         s.logger,
		aiClient:       s.aiClient,
		store:          s.store,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RealIP)
	r.Use(loggingMiddleware(s.logger))
	r.Use(recoveryMiddleware(s.logger))

	// Static file serving
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS()))))

	// Page routes
	r.Get("/", h.dashboard)
	r.Get("/messages", h.messagesList)
	r.Get("/messages/{id}", h.messageDetail)
	r.Get("/messages/{id}/body", h.messageBody)
	r.Get("/messages/{id}/body-wrapper", h.messageBodyWrapper)
	r.Get("/aggregate", h.aggregate)
	r.Get("/aggregate/drilldown", h.aggregateDrilldown)
	r.Get("/search", h.searchPage)
	r.Get("/deletions", h.deletionsPage)
	r.Post("/deletions/stage", h.stageDeletion)
	r.Delete("/deletions/{id}", h.cancelDeletion)
	r.Get("/attachments/{id}/download", h.downloadAttachment)
	r.Get("/attachments/{id}/inline", h.inlineAttachment)
	r.Get("/threads/{conversationId}", h.threadView)

	return r
}

// Start listens on the given address and serves until the context is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	r := s.buildRouter()

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
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
