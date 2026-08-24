package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := classifyError(err)
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, RequestID: requestctx.RequestID(r.Context())}})
}

func classifyError(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusBadRequest, "invalid_request", "The request is invalid."
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrExpired):
		return http.StatusUnauthorized, "unauthorized", "Authentication is required or has expired."
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden", "The current role cannot perform this action."
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found", "The requested resource was not found."
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "already_exists", "A resource with the same identity already exists."
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersion), errors.Is(err, domain.ErrCapacity), errors.Is(err, domain.ErrIllegalState):
		return http.StatusConflict, "conflict", "The request conflicts with current resource state."
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 499, "request_cancelled", "The request was cancelled."
	default:
		return http.StatusInternalServerError, "internal_error", "The service could not complete the request."
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	return nil
}
