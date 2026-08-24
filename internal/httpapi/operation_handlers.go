package httpapi

import (
	"net/http"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (a *API) reserveCapacity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ScenarioID string `json:"scenario_id"`
		SiteID     string `json:"site_id"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.ReserveCapacity(r.Context(), input.ScenarioID, input.SiteID, time.Duration(input.TTLSeconds)*time.Second)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"reservation": item})
}

func (a *API) releaseReservation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64 `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.ReleaseReservation(r.Context(), r.PathValue("id"), input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reservation": item})
}

func (a *API) createDeployment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ReservationID string `json:"reservation_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.CreateDeployment(r.Context(), input.ReservationID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deployment": item})
}

func (a *API) advanceDeployment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status   domain.DeploymentStatus `json:"status"`
		Endpoint string                  `json:"endpoint"`
		Version  int64                   `json:"version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.AdvanceDeployment(r.Context(), r.PathValue("id"), input.Status, input.Endpoint, input.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment": item})
}

func (a *API) stopDeployment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeploymentVersion  int64 `json:"deployment_version"`
		ReservationVersion int64 `json:"reservation_version"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.Service.StopDeployment(r.Context(), r.PathValue("id"), input.DeploymentVersion, input.ReservationVersion)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment": item})
}

func (a *API) reconcileExpired(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ReservationIDs []string `json:"reservation_ids"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	items, err := a.Service.ReconcileExpiredReservations(r.Context(), input.ReservationIDs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
