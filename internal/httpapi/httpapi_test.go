package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/health"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/middleware"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
)

type apiFixture struct {
	handler  http.Handler
	service  *service.Service
	operator string
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ids := &idgen.Sequence{}
	svc := service.New(database, clock.Fixed{Value: time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)}, ids, time.Hour)
	if _, err := svc.BootstrapOperator(context.Background(), "operator@example.test", "operator-password-123"); err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	checker := health.New(database)
	router := (&API{Service: svc, Health: checker}).Handler()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := middleware.Stack{Log: log, IDs: ids}.Wrap(router)
	return &apiFixture{handler: handler, service: svc}
}

func request(t *testing.T, handler http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req_http_contract")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decodeMap(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
	return payload
}

func loginToken(t *testing.T, fixture *apiFixture, email, password string) string {
	t.Helper()
	response := request(t, fixture.handler, http.MethodPost, "/v1/auth/login", map[string]string{"email": email, "password": password}, "")
	if response.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	payload := decodeMap(t, response)
	token, _ := payload["token"].(string)
	if token == "" {
		t.Fatalf("login token missing: %+v", payload)
	}
	return token
}

func TestHealthAndReadinessContracts(t *testing.T) {
	fixture := newAPIFixture(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		response := request(t, fixture.handler, http.MethodGet, path, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Request-ID") != "req_http_contract" {
			t.Fatalf("%s request id=%q", path, response.Header().Get("X-Request-ID"))
		}
		payload := decodeMap(t, response)
		if payload["status"] == "" {
			t.Fatalf("%s status payload=%+v", path, payload)
		}
	}
}

func TestRegisterLoginMeLogoutRevokesToken(t *testing.T) {
	fixture := newAPIFixture(t)
	register := request(t, fixture.handler, http.MethodPost, "/v1/auth/register", map[string]string{"email": "partner@example.test", "password": "partner-password-123"}, "")
	if register.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", register.Code, register.Body.String())
	}
	token := loginToken(t, fixture, "partner@example.test", "partner-password-123")
	me := request(t, fixture.handler, http.MethodGet, "/v1/me", nil, token)
	if me.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}
	payload := decodeMap(t, me)
	principal, ok := payload["principal"].(map[string]any)
	if !ok || principal["Role"] != "partner" {
		t.Fatalf("principal payload=%+v", payload)
	}
	logout := request(t, fixture.handler, http.MethodPost, "/v1/auth/logout", nil, token)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logout.Code, logout.Body.String())
	}
	revoked := request(t, fixture.handler, http.MethodGet, "/v1/me", nil, token)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	errorPayload := decodeMap(t, revoked)
	detail := errorPayload["error"].(map[string]any)
	if detail["code"] != "unauthorized" || detail["request_id"] != "req_http_contract" {
		t.Fatalf("unauthorized error=%+v", detail)
	}
}

func TestPartnerCannotCreateOperatorSite(t *testing.T) {
	fixture := newAPIFixture(t)
	register := request(t, fixture.handler, http.MethodPost, "/v1/auth/register", map[string]string{"email": "partner@example.test", "password": "partner-password-123"}, "")
	if register.Code != http.StatusCreated {
		t.Fatalf("register status=%d", register.Code)
	}
	token := loginToken(t, fixture, "partner@example.test", "partner-password-123")
	response := request(t, fixture.handler, http.MethodPost, "/v1/sites", map[string]any{"name": "Forbidden Site", "country_code": "SG", "timezone": "Asia/Singapore", "total_units": 8}, token)
	if response.Code != http.StatusForbidden {
		t.Fatalf("partner site status=%d body=%s", response.Code, response.Body.String())
	}
	payload := decodeMap(t, response)
	detail := payload["error"].(map[string]any)
	if detail["code"] != "forbidden" {
		t.Fatalf("error detail=%+v", detail)
	}
}

func TestOperatorCreatesSiteThroughHTTP(t *testing.T) {
	fixture := newAPIFixture(t)
	token := loginToken(t, fixture, "operator@example.test", "operator-password-123")
	response := request(t, fixture.handler, http.MethodPost, "/v1/sites", map[string]any{"name": "Manila AI Compute Exchange", "country_code": "ph", "timezone": "Asia/Manila", "total_units": 64}, token)
	if response.Code != http.StatusCreated {
		t.Fatalf("create site status=%d body=%s", response.Code, response.Body.String())
	}
	payload := decodeMap(t, response)
	site, ok := payload["site"].(map[string]any)
	if !ok {
		t.Fatalf("site response=%+v", payload)
	}
	if site["country_code"] != "PH" || site["status"] != "draft" || site["total_units"] != float64(64) {
		t.Fatalf("site contract=%+v", site)
	}
	list := request(t, fixture.handler, http.MethodGet, "/v1/sites?country=PH&limit=10&offset=0", nil, token)
	if list.Code != http.StatusOK {
		t.Fatalf("list sites status=%d body=%s", list.Code, list.Body.String())
	}
	listed := decodeMap(t, list)
	if listed["total"] != float64(1) {
		t.Fatalf("list payload=%+v", listed)
	}
}

func TestUnknownJSONFieldsAndTrailingValuesAreRejected(t *testing.T) {
	fixture := newAPIFixture(t)
	cases := []string{
		`{"email":"partner@example.test","password":"partner-password-123","role":"operator"}`,
		`{"email":"partner@example.test","password":"partner-password-123"} {}`,
		`not-json`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "req_invalid_json")
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body=%q status=%d response=%s", body, response.Code, response.Body.String())
			continue
		}
		payload := decodeMap(t, response)
		detail := payload["error"].(map[string]any)
		if detail["code"] != "invalid_request" || detail["request_id"] != "req_invalid_json" {
			t.Errorf("body=%q detail=%+v", body, detail)
		}
	}
}

func TestMissingAndMalformedBearerAreUnauthorized(t *testing.T) {
	fixture := newAPIFixture(t)
	cases := []string{"", "Basic abc", "Bearer ", "Bearer unknown"}
	for _, authorization := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		req.Header.Set("X-Request-ID", "req_auth_failure")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("authorization=%q status=%d body=%s", authorization, response.Code, response.Body.String())
		}
	}
}

func TestInvalidPaginationMapsToBadRequest(t *testing.T) {
	fixture := newAPIFixture(t)
	token := loginToken(t, fixture, "operator@example.test", "operator-password-123")
	for _, query := range []string{"limit=abc", "offset=nope"} {
		response := request(t, fixture.handler, http.MethodGet, "/v1/sites?"+query, nil, token)
		if response.Code != http.StatusBadRequest {
			t.Errorf("query=%s status=%d body=%s", query, response.Code, response.Body.String())
		}
	}
}

func TestIdempotencyReplaysSuccessfulResponseWithoutExecutingAgain(t *testing.T) {
	fixture := newAPIFixture(t)
	api := &API{Service: fixture.service}
	executions := 0
	handler := api.idempotent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions++
		writeJSON(w, http.StatusCreated, map[string]any{"reservation_id": "rsv_stable", "execution": executions})
	}))
	perform := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/reservations", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "reserve-operation-0001")
		ctx := requestctx.WithPrincipal(req.Context(), requestctx.Principal{UserID: "partner_idempotent", Role: "partner"})
		req = req.WithContext(ctx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	first := perform(`{"scenario_id":"scn_1","site_id":"site_1"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	second := perform(`{"scenario_id":"scn_1","site_id":"site_1"}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("replay status=%d body=%s", second.Code, second.Body.String())
	}
	if second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay header=%q", second.Header().Get("Idempotency-Replayed"))
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body differs first=%s second=%s", first.Body.String(), second.Body.String())
	}
	if executions != 1 {
		t.Fatalf("handler executions=%d", executions)
	}
}

func TestIdempotencyRejectsPayloadChangesAndMissingKeys(t *testing.T) {
	fixture := newAPIFixture(t)
	api := &API{Service: fixture.service}
	handler := api.idempotent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]bool{"created": true})
	}))
	perform := func(body, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/deployments", strings.NewReader(body))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		ctx := requestctx.WithPrincipal(req.Context(), requestctx.Principal{UserID: "partner_idempotent", Role: "partner"})
		req = req.WithContext(ctx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := perform(`{"reservation_id":"rsv_1"}`, "deploy-operation-0001"); response.Code != http.StatusCreated {
		t.Fatalf("initial status=%d body=%s", response.Code, response.Body.String())
	}
	changed := perform(`{"reservation_id":"rsv_2"}`, "deploy-operation-0001")
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed payload status=%d body=%s", changed.Code, changed.Body.String())
	}
	missing := perform(`{"reservation_id":"rsv_1"}`, "")
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing key status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestIdempotencyFailureAbandonsClaimForRetry(t *testing.T) {
	fixture := newAPIFixture(t)
	api := &API{Service: fixture.service}
	executions := 0
	handler := api.idempotent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions++
		if executions == 1 {
			writeError(w, r, domain.ErrConflict)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]bool{"created": true})
	}))
	perform := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/reservations", strings.NewReader(`{"scenario_id":"scn_retry"}`))
		req.Header.Set("Idempotency-Key", "reserve-operation-retry")
		ctx := requestctx.WithPrincipal(req.Context(), requestctx.Principal{UserID: "partner_retry", Role: "partner"})
		req = req.WithContext(ctx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if first := perform(); first.Code != http.StatusConflict {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	if second := perform(); second.Code != http.StatusCreated {
		t.Fatalf("retry status=%d body=%s", second.Code, second.Body.String())
	}
	if executions != 2 {
		t.Fatalf("executions=%d", executions)
	}
}
