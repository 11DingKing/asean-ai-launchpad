package httpapi

import (
	"bytes"
	"io"
	"net/http"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

const maxIdempotentBody = 1 << 20

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header)}
}

func (w *bufferedResponse) Header() http.Header { return w.header }

func (w *bufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *bufferedResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (a *API) idempotent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, r, domain.ErrInvalid)
			return
		}
		principal, ok := requestctx.PrincipalFrom(r.Context())
		if !ok {
			writeError(w, r, domain.ErrUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotentBody+1))
		if err != nil || len(body) > maxIdempotentBody {
			writeError(w, r, domain.ErrInvalid)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		record, claimed, err := a.Service.ClaimIdempotency(r.Context(), principal.UserID, r.Method, r.URL.Path, key, body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if !claimed {
			if record.ResponseStatus == 0 {
				writeError(w, r, domain.ErrConflict)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(record.ResponseStatus)
			_, _ = io.WriteString(w, record.ResponseBody)
			return
		}
		buffer := newBufferedResponse()
		next.ServeHTTP(buffer, r)
		status := buffer.status
		if status == 0 {
			status = http.StatusOK
		}
		if status >= 200 && status < 300 {
			if err := a.Service.CompleteIdempotency(r.Context(), record.Scope, status, buffer.body.String()); err != nil {
				writeError(w, r, err)
				return
			}
		} else {
			_ = a.Service.AbandonIdempotency(r.Context(), record.Scope)
		}
		for name, values := range buffer.header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write(buffer.body.Bytes())
	})
}
