package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

func TestRequestIDReachesHandlerResponseAndLog(t *testing.T) {
	var logs bytes.Buffer
	stack := Stack{Log: slog.New(slog.NewJSONHandler(&logs, nil)), IDs: &idgen.Sequence{}}
	handler := stack.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestctx.RequestID(r.Context()); got != "req_observable" {
			t.Fatalf("handler request id=%q", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/example", nil)
	req.Header.Set("X-Request-ID", "req_observable")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Header().Get("X-Request-ID"); got != "req_observable" {
		t.Fatalf("response request id=%q", got)
	}
	if !strings.Contains(logs.String(), `"request_id":"req_observable"`) {
		t.Fatalf("request id absent from log: %s", logs.String())
	}
}

func TestRequestIDIsGeneratedWhenMissingOrOversized(t *testing.T) {
	stack := Stack{Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), IDs: &idgen.Sequence{}}
	seen := make([]string, 0, 2)
	handler := stack.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, requestctx.RequestID(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, supplied := range []string{"", strings.Repeat("x", 129)} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		if supplied != "" {
			req.Header.Set("X-Request-ID", supplied)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d", response.Code)
		}
		if !strings.HasPrefix(response.Header().Get("X-Request-ID"), "req_") {
			t.Fatalf("generated response id=%q", response.Header().Get("X-Request-ID"))
		}
	}
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("generated ids=%v", seen)
	}
}
