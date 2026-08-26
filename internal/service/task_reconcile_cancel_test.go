package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

type cancellingReservationStore struct {
	repository.Store
	cancel  context.CancelFunc
	finds   int
	expired []string
}

func (s *cancellingReservationStore) FindReservation(_ context.Context, stringID string) (domain.Reservation, error) {
	s.finds++
	if s.finds == 1 {
		s.cancel()
	}
	return domain.Reservation{ID: stringID, SiteID: "site_1", Units: 2, Status: domain.ReservationConfirmed, ExpiresAt: time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC), Version: 1}, nil
}

func (s *cancellingReservationStore) ExpireReservation(_ context.Context, item domain.Reservation, _ domain.AuditEvent) error {
	s.expired = append(s.expired, item.ID)
	return nil
}

func TestCancelledReconciliationStopsBeforeNextReservation(t *testing.T) {
	ctx, cancel := context.WithCancel(requestctx.WithPrincipal(context.Background(), requestctx.Principal{UserID: "operator_1", Role: "operator"}))
	store := &cancellingReservationStore{cancel: cancel}
	svc := New(store, clock.Fixed{Value: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)}, &idgen.Sequence{}, time.Hour)

	results, err := svc.ReconcileExpiredReservations(ctx, []string{"rsv_first", "rsv_second", "rsv_third"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reconciliation did not report cancellation: results=%+v err=%v", results, err)
	}
	if len(results) != 1 || len(store.expired) != 1 || store.expired[0] != "rsv_first" {
		t.Fatalf("cancellation allowed later side effects: results=%+v expired=%v", results, store.expired)
	}
}
