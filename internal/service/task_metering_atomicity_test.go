package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
)

var errSettlementUnavailable = errors.New("settlement ledger unavailable")

type failingSettlementStore struct {
	repository.Store
}

func (failingSettlementStore) RecordSettlement(context.Context, domain.SettlementEntry, domain.AuditEvent) error {
	return errSettlementUnavailable
}

func TestFailedSettlementDoesNotLeaveUsageRecord(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 8)
	scenario := fixture.approvedScenario(t, 2)
	reservation, err := fixture.service.ReserveCapacity(fixture.partCtx, scenario.ID, site.ID, time.Hour)
	if err != nil {
		t.Fatalf("reserve capacity: %v", err)
	}
	deployment, err := fixture.service.CreateDeployment(fixture.partCtx, reservation.ID)
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := fixture.service.ActivateDeploymentJob(context.Background(), deployment.ID); err != nil {
		t.Fatalf("activate deployment: %v", err)
	}

	fixture.service.Store = failingSettlementStore{Store: fixture.store}
	periodStart := fixture.clock.Now().Add(-30 * time.Minute)
	periodEnd := fixture.clock.Now()
	_, _, err = fixture.service.RecordUsage(fixture.opCtx, deployment.ID, fixture.partner.ID, periodStart, periodEnd, 3_600, "USD")
	if !errors.Is(err, errSettlementUnavailable) {
		t.Fatalf("record usage should surface settlement failure, got %v", err)
	}

	items, total, err := fixture.service.ListUsage(fixture.partCtx, periodStart.Add(-time.Minute), periodEnd.Add(time.Minute), domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("list usage after failed settlement: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("failed settlement left unbillable usage: total=%d items=%+v", total, items)
	}

	_, _, err = fixture.service.RecordUsage(fixture.opCtx, deployment.ID, fixture.partner.ID, periodStart, periodEnd, 3_600, "USD")
	if !errors.Is(err, errSettlementUnavailable) {
		t.Fatalf("same period should remain retryable after ledger recovery, got %v", err)
	}
}
