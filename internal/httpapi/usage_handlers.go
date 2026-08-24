package httpapi

import (
	"net/http"
	"time"
)

func (a *API) recordUsage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeploymentID string    `json:"deployment_id"`
		PartnerID    string    `json:"partner_id"`
		PeriodStart  time.Time `json:"period_start"`
		PeriodEnd    time.Time `json:"period_end"`
		UnitSeconds  int64     `json:"unit_seconds"`
		Currency     string    `json:"currency"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	usage, settlement, err := a.Service.RecordUsage(r.Context(), input.DeploymentID, input.PartnerID, input.PeriodStart, input.PeriodEnd, input.UnitSeconds, input.Currency)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"usage": usage, "settlement": settlement})
}

func (a *API) listUsage(w http.ResponseWriter, r *http.Request) {
	from, err := timeQuery(r, "from")
	if err != nil {
		writeError(w, r, err)
		return
	}
	to, err := timeQuery(r, "to")
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := pageFrom(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := a.Service.ListUsage(r.Context(), from, to, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset})
}
