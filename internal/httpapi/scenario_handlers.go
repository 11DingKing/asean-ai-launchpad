package httpapi

import (
	"net/http"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
)

func (a *API) submitScenario(w http.ResponseWriter, r *http.Request) {
	var input service.SubmitScenarioInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.SubmitScenario(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"scenario": item})
}

func (a *API) listScenarios(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := a.Service.ListScenarios(r.Context(), domain.ScenarioStatus(r.URL.Query().Get("status")), page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset})
}

func (a *API) startScenarioReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64 `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.StartScenarioReview(r.Context(), r.PathValue("id"), input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scenario": item})
}

func (a *API) decideScenario(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
		Version  int64  `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.DecideScenario(r.Context(), r.PathValue("id"), input.Decision, input.Notes, input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scenario": item})
}
