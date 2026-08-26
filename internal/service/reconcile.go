package service

import (
	"context"
	"errors"
)

type ReconcileResult struct {
	ID      string `json:"id"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) ReconcileExpiredReservations(ctx context.Context, ids []string) ([]ReconcileResult, error) {
	if _, err := requireRole(ctx, "operator"); err != nil {
		return nil, err
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, errors.New("reconciliation requires 1 to 100 reservation ids")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results := make([]ReconcileResult, 0, len(ids))
	processingCtx := context.WithoutCancel(ctx)
	for _, id := range ids {
		result := ReconcileResult{ID: id}
		if err := s.ExpireReservation(processingCtx, id); err != nil {
			result.Error = err.Error()
		} else {
			result.Applied = true
		}
		results = append(results, result)
	}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}
