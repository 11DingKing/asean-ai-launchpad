package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Service) ReserveCapacity(ctx context.Context, scenarioID, siteID string, ttl time.Duration) (domain.Reservation, error) {
	actor, err := requireRole(ctx, domain.RolePartner)
	if err != nil {
		return domain.Reservation{}, err
	}
	scenario, err := s.Store.FindScenario(ctx, scenarioID)
	if err != nil {
		return domain.Reservation{}, err
	}
	site, err := s.Store.FindSite(ctx, siteID)
	if err != nil {
		return domain.Reservation{}, err
	}
	id, err := s.IDs.New("rsv")
	if err != nil {
		return domain.Reservation{}, err
	}
	item, err := domain.NewReservation(id, scenario, site, actor.UserID, ttl, s.Clock.Now())
	if err != nil {
		return domain.Reservation{}, err
	}
	payload, err := json.Marshal(map[string]any{"reservation_id": item.ID, "expires_at": item.ExpiresAt})
	if err != nil {
		return domain.Reservation{}, err
	}
	job, err := s.job("expire_reservation", item.ID, string(payload), item.ExpiresAt)
	if err != nil {
		return domain.Reservation{}, err
	}
	event, err := s.event(ctx, actor.UserID, "reservation.create", "reservation", item.ID, "success", map[string]any{"scenario_id": scenario.ID, "site_id": site.ID, "units": item.Units})
	if err != nil {
		return domain.Reservation{}, err
	}
	if err := s.Store.CreateReservation(ctx, item, job, event); err != nil {
		return domain.Reservation{}, err
	}
	return item, nil
}

func (s *Service) ReleaseReservation(ctx context.Context, id string, version int64) (domain.Reservation, error) {
	actor, err := requireRole(ctx, domain.RolePartner)
	if err != nil {
		return domain.Reservation{}, err
	}
	item, err := s.Store.FindReservation(ctx, id)
	if err != nil {
		return domain.Reservation{}, err
	}
	if item.PartnerID != actor.UserID {
		return domain.Reservation{}, domain.ErrForbidden
	}
	if item.Version != version {
		return domain.Reservation{}, domain.ErrVersion
	}
	if err := item.Transition(domain.ReservationReleased, s.Clock.Now()); err != nil {
		return domain.Reservation{}, err
	}
	event, err := s.event(ctx, actor.UserID, "reservation.release", "reservation", item.ID, "success", map[string]any{"version": version, "units": item.Units})
	if err != nil {
		return domain.Reservation{}, err
	}
	if err := s.Store.ReleaseReservation(ctx, item, event); err != nil {
		return domain.Reservation{}, err
	}
	return item, nil
}

func (s *Service) ExpireReservation(ctx context.Context, id string) error {
	ctx = context.WithoutCancel(ctx)
	item, err := s.Store.FindReservation(ctx, id)
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	if item.Status == domain.ReservationReleased || item.Status == domain.ReservationExpired {
		return nil
	}
	if now.Before(item.ExpiresAt) {
		return domain.ErrConflict
	}
	if err := item.Transition(domain.ReservationExpired, now); err != nil {
		return err
	}
	event, err := s.event(ctx, "", "reservation.expire", "reservation", item.ID, "success", map[string]any{"units": item.Units})
	if err != nil {
		return err
	}
	return s.Store.ExpireReservation(ctx, item, event)
}
