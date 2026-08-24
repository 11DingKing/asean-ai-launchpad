package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/health"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
)

type API struct {
	Service *service.Service
	Health  *health.Checker
}

type sessionKey struct{}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /readyz", a.ready)
	mux.HandleFunc("POST /v1/auth/register", a.register)
	mux.HandleFunc("POST /v1/auth/login", a.login)
	mux.Handle("POST /v1/auth/logout", a.auth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /v1/me", a.auth(http.HandlerFunc(a.me)))
	mux.Handle("POST /v1/sites", a.auth(http.HandlerFunc(a.createSite)))
	mux.Handle("GET /v1/sites", a.auth(http.HandlerFunc(a.listSites)))
	mux.Handle("POST /v1/sites/{id}/reviews", a.auth(http.HandlerFunc(a.reviewSite)))
	mux.Handle("POST /v1/sites/{id}/transitions", a.auth(http.HandlerFunc(a.transitionSite)))
	mux.Handle("POST /v1/scenarios", a.auth(http.HandlerFunc(a.submitScenario)))
	mux.Handle("GET /v1/scenarios", a.auth(http.HandlerFunc(a.listScenarios)))
	mux.Handle("POST /v1/scenarios/{id}/review-start", a.auth(http.HandlerFunc(a.startScenarioReview)))
	mux.Handle("POST /v1/scenarios/{id}/decisions", a.auth(http.HandlerFunc(a.decideScenario)))
	mux.Handle("POST /v1/reservations", a.auth(a.idempotent(http.HandlerFunc(a.reserveCapacity))))
	mux.Handle("POST /v1/reservations/{id}/release", a.auth(http.HandlerFunc(a.releaseReservation)))
	mux.Handle("POST /v1/deployments", a.auth(a.idempotent(http.HandlerFunc(a.createDeployment))))
	mux.Handle("POST /v1/deployments/{id}/transitions", a.auth(http.HandlerFunc(a.advanceDeployment)))
	mux.Handle("POST /v1/deployments/{id}/stop", a.auth(http.HandlerFunc(a.stopDeployment)))
	mux.Handle("POST /v1/usage", a.auth(http.HandlerFunc(a.recordUsage)))
	mux.Handle("GET /v1/usage", a.auth(http.HandlerFunc(a.listUsage)))
	mux.Handle("POST /v1/operations/reconcile-expired", a.auth(http.HandlerFunc(a.reconcileExpired)))
	return mux
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, r, domain.ErrUnauthorized)
			return
		}
		principal, sessionID, err := a.Service.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(w, r, err)
			return
		}
		ctx := requestctx.WithPrincipal(r.Context(), principal)
		ctx = context.WithValue(ctx, sessionKey{}, sessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if !a.Health.Live() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "stopping"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := a.Health.Ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
