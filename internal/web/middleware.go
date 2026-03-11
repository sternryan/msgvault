package web

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// loggingMiddleware logs each request with method, path, status, and duration.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			logger.Debug("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", time.Since(start).Round(time.Microsecond),
			)
		})
	}
}

// recoveryMiddleware catches panics and renders an HTML 500 error page.
func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"stack", string(debug.Stack()),
					)
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`<!DOCTYPE html><html><body><h1>500 Internal Server Error</h1><p>An unexpected error occurred.</p><a href="/">Return to Dashboard</a></body></html>`)) //nolint:errcheck
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
