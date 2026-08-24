package httpapi

import (
	"net/http"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
)

func (a *API) createSite(w http.ResponseWriter, r *http.Request) {
	var input service.CreateSiteInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.CreateSite(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"site": item})
}

func (a *API) listSites(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := a.Service.ListSites(r.Context(), r.URL.Query().Get("country"), domain.SiteStatus(r.URL.Query().Get("status")), page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset})
}

func (a *API) reviewSite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision    string `json:"decision"`
		EvidenceRef string `json:"evidence_ref"`
		Version     int64  `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.ReviewSite(r.Context(), r.PathValue("id"), input.Decision, input.EvidenceRef, input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": item})
}

func (a *API) transitionSite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status  domain.SiteStatus `json:"status"`
		Version int64             `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.TransitionSite(r.Context(), r.PathValue("id"), input.Status, input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": item})
}
