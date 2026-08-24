package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Service) CreateDeployment(ctx context.Context, reservationID string) (domain.Deployment, error) {
	actor, err := requireRole(ctx, domain.RolePartner)
	if err != nil {
		return domain.Deployment{}, err
	}
	reservation, err := s.Store.FindReservation(ctx, reservationID)
	if err != nil {
		return domain.Deployment{}, err
	}
	if reservation.PartnerID != actor.UserID {
		return domain.Deployment{}, domain.ErrForbidden
	}
	id, err := s.IDs.New("dep")
	if err != nil {
		return domain.Deployment{}, err
	}
	item, err := domain.NewDeployment(id, reservation, s.Clock.Now())
	if err != nil {
		return domain.Deployment{}, err
	}
	payload, err := json.Marshal(map[string]string{"deployment_id": item.ID})
	if err != nil {
		return domain.Deployment{}, err
	}
	job, err := s.job("activate_deployment", item.ID, string(payload), s.Clock.Now())
	if err != nil {
		return domain.Deployment{}, err
	}
	event, err := s.event(ctx, actor.UserID, "deployment.create", "deployment", item.ID, "success", map[string]string{"reservation_id": reservationID})
	if err != nil {
		return domain.Deployment{}, err
	}
	if err := s.Store.CreateDeployment(ctx, item, job, event); err != nil {
		return domain.Deployment{}, err
	}
	return item, nil
}

func (s *Service) AdvanceDeployment(ctx context.Context, id string, next domain.DeploymentStatus, endpoint string, expectedVersion int64) (domain.Deployment, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.Deployment{}, err
	}
	item, err := s.Store.FindDeployment(ctx, id)
	if err != nil {
		return domain.Deployment{}, err
	}
	if item.Version != expectedVersion {
		return domain.Deployment{}, domain.ErrVersion
	}
	if next == domain.DeploymentRunning {
		if endpoint, err = requireText(endpoint, "endpoint", 8, 500); err != nil {
			return domain.Deployment{}, err
		}
	}
	if err := item.Transition(next, endpoint, s.Clock.Now()); err != nil {
		return domain.Deployment{}, err
	}
	event, err := s.event(ctx, actor.UserID, "deployment.transition", "deployment", item.ID, "success", map[string]any{"status": next, "version": expectedVersion})
	if err != nil {
		return domain.Deployment{}, err
	}
	if err := s.Store.TransitionDeployment(ctx, item, event); err != nil {
		return domain.Deployment{}, err
	}
	return item, nil
}

func (s *Service) StopDeployment(ctx context.Context, id string, deploymentVersion, reservationVersion int64) (domain.Deployment, error) {
	actor, err := requireRole(ctx, domain.RoleOperator)
	if err != nil {
		return domain.Deployment{}, err
	}
	item, err := s.Store.FindDeployment(ctx, id)
	if err != nil {
		return domain.Deployment{}, err
	}
	if item.Version != deploymentVersion {
		return domain.Deployment{}, domain.ErrVersion
	}
	reservation, err := s.Store.FindReservation(ctx, item.ReservationID)
	if err != nil {
		return domain.Deployment{}, err
	}
	if reservation.Version != reservationVersion {
		return domain.Deployment{}, domain.ErrVersion
	}
	if item.Status == domain.DeploymentRunning {
		if err := item.Transition(domain.DeploymentStopping, "", s.Clock.Now()); err != nil {
			return domain.Deployment{}, err
		}
		event, err := s.event(ctx, actor.UserID, "deployment.stop.start", "deployment", item.ID, "success", map[string]any{"version": deploymentVersion})
		if err != nil {
			return domain.Deployment{}, err
		}
		if err := s.Store.TransitionDeployment(ctx, item, event); err != nil {
			return domain.Deployment{}, err
		}
		return item, nil
	}
	if item.Status == domain.DeploymentStopping {
		if err := item.Transition(domain.DeploymentStopped, "", s.Clock.Now()); err != nil {
			return domain.Deployment{}, err
		}
	} else {
		return domain.Deployment{}, fmt.Errorf("%w: deployment cannot stop from %s", domain.ErrIllegalState, item.Status)
	}
	if err := reservation.Transition(domain.ReservationReleased, s.Clock.Now()); err != nil {
		return domain.Deployment{}, err
	}
	event, err := s.event(ctx, actor.UserID, "deployment.stop", "deployment", item.ID, "success", map[string]any{"reservation_id": reservation.ID, "units": reservation.Units})
	if err != nil {
		return domain.Deployment{}, err
	}
	if err := s.Store.StopDeploymentAndRelease(ctx, item, reservation, event); err != nil {
		return domain.Deployment{}, err
	}
	return item, nil
}

func (s *Service) ActivateDeploymentJob(ctx context.Context, id string) error {
	ctx = context.WithoutCancel(ctx)
	item, err := s.Store.FindDeployment(ctx, id)
	if err != nil {
		return err
	}
	if item.Status == domain.DeploymentRunning {
		return nil
	}
	if item.Status == domain.DeploymentQueued {
		if err := item.Transition(domain.DeploymentActivating, "", s.Clock.Now()); err != nil {
			return err
		}
		event, err := s.event(ctx, "", "deployment.activate.start", "deployment", item.ID, "success", map[string]any{"version": item.Version})
		if err != nil {
			return err
		}
		if err := s.Store.TransitionDeployment(ctx, item, event); err != nil {
			return err
		}
	}
	if item.Status != domain.DeploymentActivating {
		return fmt.Errorf("%w: deployment job state %s", domain.ErrIllegalState, item.Status)
	}
	endpoint := "https://runtime.local/deployments/" + item.ID
	if err := item.Transition(domain.DeploymentRunning, endpoint, s.Clock.Now()); err != nil {
		return err
	}
	event, err := s.event(ctx, "", "deployment.activate.complete", "deployment", item.ID, "success", map[string]string{"endpoint": endpoint})
	if err != nil {
		return err
	}
	return s.Store.TransitionDeployment(ctx, item, event)
}
