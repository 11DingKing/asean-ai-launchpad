package httpapi

import (
	"net/http"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	user, err := a.Service.RegisterPartner(r.Context(), input.Email, input.Password)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.Service.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	sessionID, _ := r.Context().Value(sessionKey{}).(string)
	if err := a.Service.Logout(r.Context(), sessionID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestctx.PrincipalFrom(r.Context())
	if !ok {
		writeError(w, r, domain.ErrUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"principal": principal})
}
