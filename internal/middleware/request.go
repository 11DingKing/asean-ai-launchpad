package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

type Stack struct {
	Log *slog.Logger
	IDs idgen.Generator
}

func (s Stack) Wrap(next http.Handler) http.Handler {
	return s.requestID(s.recoverPanic(s.logRequest(next)))
}

func (s Stack) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 128 {
			generated, err := s.IDs.New("req")
			if err != nil {
				http.Error(w, "request id unavailable", http.StatusServiceUnavailable)
				return
			}
			id = generated
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(requestctx.WithRequestID(r.Context(), id)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (s Stack) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		status := wrapped.status
		if status == 0 {
			status = http.StatusOK
		}
		s.Log.InfoContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "status", status, "bytes", wrapped.bytes, "duration_ms", time.Since(started).Milliseconds(), "request_id", requestctx.RequestID(r.Context()))
	})
}

func (s Stack) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.Log.ErrorContext(r.Context(), "panic recovered", "panic", recovered, "stack", string(debug.Stack()), "request_id", requestctx.RequestID(r.Context()))
				if !headersWritten(w) {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func headersWritten(w http.ResponseWriter) bool {
	if wrapped, ok := w.(*statusWriter); ok {
		return wrapped.status != 0
	}
	return false
}
