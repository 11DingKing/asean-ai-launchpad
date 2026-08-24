package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
)

var errCapacityReturnUnavailable = errors.New("capacity return unavailable")

type failingFinalStopStore struct {
	repository.Store
	fail bool
}

func (s *failingFinalStopStore) ReleaseStoppedDeploymentCapacity(ctx context.Context, reservation domain.Reservation, event domain.AuditEvent) error {
	if s.fail {
		s.fail = false
		return errCapacityReturnUnavailable
	}
	split, ok := s.Store.(interface {
		ReleaseStoppedDeploymentCapacity(context.Context, domain.Reservation, domain.AuditEvent) error
	})
	if !ok {
		return errors.New("split capacity operation unavailable")
	}
	return split.ReleaseStoppedDeploymentCapacity(ctx, reservation, event)
}

func (s *failingFinalStopStore) StopDeploymentAndRelease(ctx context.Context, deployment domain.Deployment, reservation domain.Reservation, event domain.AuditEvent) error {
	if s.fail {
		s.fail = false
		return errCapacityReturnUnavailable
	}
	atomic, ok := s.Store.(interface {
		StopDeploymentAndRelease(context.Context, domain.Deployment, domain.Reservation, domain.AuditEvent) error
	})
	if !ok {
		return errors.New("atomic stop operation unavailable")
	}
	return atomic.StopDeploymentAndRelease(ctx, deployment, reservation, event)
}

func TestFailedFinalStopKeepsDeploymentAndCapacityTogether(t *testing.T) {
	fixture := newServiceFixture(t)
	site := fixture.activeSite(t, 10)
	scenario := fixture.approvedScenario(t, 3)
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
	deployment, err = fixture.store.FindDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatalf("find running deployment: %v", err)
	}
	stopping, err := fixture.service.StopDeployment(fixture.opCtx, deployment.ID, deployment.Version, reservation.Version)
	if err != nil || stopping.Status != domain.DeploymentStopping {
		t.Fatalf("enter stopping state: deployment=%+v err=%v", stopping, err)
	}

	failureStore := &failingFinalStopStore{Store: fixture.store, fail: true}
	fixture.service.Store = failureStore
	_, err = fixture.service.StopDeployment(fixture.opCtx, deployment.ID, stopping.Version, reservation.Version)
	if !errors.Is(err, errCapacityReturnUnavailable) {
		t.Fatalf("final stop should surface capacity failure, got %v", err)
	}
	persistedDeployment, err := fixture.store.FindDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatalf("find deployment after failure: %v", err)
	}
	persistedReservation, err := fixture.store.FindReservation(context.Background(), reservation.ID)
	if err != nil {
		t.Fatalf("find reservation after failure: %v", err)
	}
	persistedSite, err := fixture.store.FindSite(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("find site after failure: %v", err)
	}
	if persistedDeployment.Status != domain.DeploymentStopping || persistedReservation.Status != domain.ReservationConfirmed || persistedSite.AvailableUnits != site.TotalUnits-reservation.Units {
		t.Errorf("failed stop split resource state: deployment=%s reservation=%s available=%d", persistedDeployment.Status, persistedReservation.Status, persistedSite.AvailableUnits)
	}

	stopped, err := fixture.service.StopDeployment(fixture.opCtx, deployment.ID, stopping.Version, reservation.Version)
	if err != nil || stopped.Status != domain.DeploymentStopped {
		t.Fatalf("retry after capacity recovery: deployment=%+v err=%v", stopped, err)
	}
	persistedReservation, _ = fixture.store.FindReservation(context.Background(), reservation.ID)
	persistedSite, _ = fixture.store.FindSite(context.Background(), site.ID)
	if persistedReservation.Status != domain.ReservationReleased || persistedSite.AvailableUnits != site.TotalUnits {
		t.Fatalf("successful retry did not release resources: reservation=%s available=%d", persistedReservation.Status, persistedSite.AvailableUnits)
	}
}
