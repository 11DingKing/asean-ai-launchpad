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
	results := make([]ReconcileResult, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result := ReconcileResult{ID: id}
		if err := s.ExpireReservation(ctx, id); err != nil {
			result.Error = err.Error()
		} else {
			result.Applied = true
		}
		results = append(results, result)
	}
	return results, nil
}
