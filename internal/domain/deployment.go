package domain

import (
	"fmt"
	"time"
)

type DeploymentStatus string

const (
	DeploymentQueued     DeploymentStatus = "queued"
	DeploymentActivating DeploymentStatus = "activating"
	DeploymentRunning    DeploymentStatus = "running"
	DeploymentStopping   DeploymentStatus = "stopping"
	DeploymentStopped    DeploymentStatus = "stopped"
	DeploymentFailed     DeploymentStatus = "failed"
)

type Deployment struct {
	ID            string           `json:"id"`
	ReservationID string           `json:"reservation_id"`
	Status        DeploymentStatus `json:"status"`
	Endpoint      string           `json:"endpoint"`
	Version       int64            `json:"version"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	StoppedAt     *time.Time       `json:"stopped_at,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func NewDeployment(id string, reservation Reservation, now time.Time) (Deployment, error) {
	if id == "" {
		return Deployment{}, fmt.Errorf("%w: deployment id", ErrInvalid)
	}
	if err := reservation.Deployable(now); err != nil {
		return Deployment{}, err
	}
	now = now.UTC()
	return Deployment{ID: id, ReservationID: reservation.ID, Status: DeploymentQueued, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (d Deployment) CanTransition(next DeploymentStatus) bool {
	switch d.Status {
	case DeploymentQueued:
		return next == DeploymentActivating || next == DeploymentFailed
	case DeploymentActivating:
		return next == DeploymentRunning || next == DeploymentFailed
	case DeploymentRunning:
		return next == DeploymentStopping || next == DeploymentFailed
	case DeploymentStopping:
		return next == DeploymentStopped || next == DeploymentFailed
	default:
		return false
	}
}

func (d *Deployment) Transition(next DeploymentStatus, endpoint string, now time.Time) error {
	if !d.CanTransition(next) {
		return fmt.Errorf("%w: deployment %s to %s", ErrIllegalState, d.Status, next)
	}
	now = now.UTC()
	d.Status = next
	d.Version++
	d.UpdatedAt = now
	if next == DeploymentRunning {
		d.Endpoint = endpoint
		d.StartedAt = &now
	}
	if next == DeploymentStopped {
		d.StoppedAt = &now
	}
	return nil
}

type UsageRecord struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	PartnerID    string    `json:"partner_id"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	UnitSeconds  int64     `json:"unit_seconds"`
	CreatedAt    time.Time `json:"created_at"`
}

func NewUsageRecord(id, deploymentID, partnerID string, start, end time.Time, unitSeconds int64, now time.Time) (UsageRecord, error) {
	if id == "" || deploymentID == "" || partnerID == "" || !end.After(start) || unitSeconds < 0 {
		return UsageRecord{}, fmt.Errorf("%w: usage record", ErrInvalid)
	}
	if end.Sub(start) > 31*24*time.Hour {
		return UsageRecord{}, fmt.Errorf("%w: usage period too long", ErrInvalid)
	}
	return UsageRecord{ID: id, DeploymentID: deploymentID, PartnerID: partnerID, PeriodStart: start.UTC(), PeriodEnd: end.UTC(), UnitSeconds: unitSeconds, CreatedAt: now.UTC()}, nil
}

type SettlementEntry struct {
	ID           string    `json:"id"`
	UsageID      string    `json:"usage_id"`
	PartnerID    string    `json:"partner_id"`
	AmountMicros int64     `json:"amount_micros"`
	Currency     string    `json:"currency"`
	CreatedAt    time.Time `json:"created_at"`
}
