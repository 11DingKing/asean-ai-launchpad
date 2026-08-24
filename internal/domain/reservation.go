package domain

import (
	"fmt"
	"time"
)

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "pending"
	ReservationConfirmed ReservationStatus = "confirmed"
	ReservationReleased  ReservationStatus = "released"
	ReservationExpired   ReservationStatus = "expired"
)

type Reservation struct {
	ID         string            `json:"id"`
	ScenarioID string            `json:"scenario_id"`
	SiteID     string            `json:"site_id"`
	PartnerID  string            `json:"partner_id"`
	Units      int               `json:"units"`
	Status     ReservationStatus `json:"status"`
	ExpiresAt  time.Time         `json:"expires_at"`
	Version    int64             `json:"version"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

func NewReservation(id string, scenario Scenario, site ComputeSite, partnerID string, ttl time.Duration, now time.Time) (Reservation, error) {
	if id == "" || partnerID == "" || scenario.PartnerID != partnerID {
		return Reservation{}, fmt.Errorf("%w: reservation owner", ErrForbidden)
	}
	if scenario.Status != ScenarioApproved {
		return Reservation{}, fmt.Errorf("%w: scenario is %s", ErrIllegalState, scenario.Status)
	}
	if err := site.CanReserve(scenario.RequestedUnits); err != nil {
		return Reservation{}, err
	}
	if ttl < time.Minute || ttl > 7*24*time.Hour {
		return Reservation{}, fmt.Errorf("%w: reservation ttl", ErrInvalid)
	}
	now = now.UTC()
	return Reservation{ID: id, ScenarioID: scenario.ID, SiteID: site.ID, PartnerID: partnerID, Units: scenario.RequestedUnits, Status: ReservationConfirmed, ExpiresAt: now.Add(ttl), Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (r Reservation) CanTransition(next ReservationStatus) bool {
	switch r.Status {
	case ReservationPending:
		return next == ReservationConfirmed || next == ReservationReleased || next == ReservationExpired
	case ReservationConfirmed:
		return next == ReservationReleased || next == ReservationExpired
	default:
		return false
	}
}

func (r *Reservation) Transition(next ReservationStatus, now time.Time) error {
	if !r.CanTransition(next) {
		return fmt.Errorf("%w: reservation %s to %s", ErrIllegalState, r.Status, next)
	}
	r.Status = next
	r.Version++
	r.UpdatedAt = now.UTC()
	return nil
}

func (r Reservation) Deployable(at time.Time) error {
	if r.Status != ReservationConfirmed {
		return fmt.Errorf("%w: reservation is %s", ErrIllegalState, r.Status)
	}
	if !at.Before(r.ExpiresAt) {
		return ErrExpired
	}
	return nil
}
