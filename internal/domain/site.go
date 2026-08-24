package domain

import (
	"fmt"
	"strings"
	"time"
)

type SiteStatus string

const (
	SiteDraft     SiteStatus = "draft"
	SiteVerified  SiteStatus = "verified"
	SiteActive    SiteStatus = "active"
	SiteSuspended SiteStatus = "suspended"
)

type ComputeSite struct {
	ID             string     `json:"id"`
	OperatorID     string     `json:"operator_id"`
	Name           string     `json:"name"`
	CountryCode    string     `json:"country_code"`
	Timezone       string     `json:"timezone"`
	TotalUnits     int        `json:"total_units"`
	AvailableUnits int        `json:"available_units"`
	Status         SiteStatus `json:"status"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func NewComputeSite(id, operatorID, name, country, timezone string, units int, now time.Time) (ComputeSite, error) {
	name = strings.TrimSpace(name)
	country = strings.ToUpper(strings.TrimSpace(country))
	if id == "" || operatorID == "" || len(name) < 3 || len(name) > 120 {
		return ComputeSite{}, fmt.Errorf("%w: site identity", ErrInvalid)
	}
	if len(country) != 2 || timezone == "" || units <= 0 || units > 1_000_000 {
		return ComputeSite{}, fmt.Errorf("%w: site capacity or location", ErrInvalid)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return ComputeSite{}, fmt.Errorf("%w: timezone", ErrInvalid)
	}
	now = now.UTC()
	return ComputeSite{ID: id, OperatorID: operatorID, Name: name, CountryCode: country, Timezone: timezone, TotalUnits: units, AvailableUnits: units, Status: SiteDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (s ComputeSite) CanTransition(next SiteStatus) bool {
	switch s.Status {
	case SiteDraft:
		return next == SiteVerified || next == SiteSuspended
	case SiteVerified:
		return next == SiteActive || next == SiteSuspended
	case SiteActive:
		return next == SiteSuspended
	case SiteSuspended:
		return next == SiteActive
	default:
		return false
	}
}

func (s *ComputeSite) Transition(next SiteStatus, now time.Time) error {
	if !s.CanTransition(next) {
		return fmt.Errorf("%w: site %s to %s", ErrIllegalState, s.Status, next)
	}
	s.Status = next
	s.Version++
	s.UpdatedAt = now.UTC()
	return nil
}

func (s ComputeSite) CanReserve(units int) error {
	if s.Status != SiteActive {
		return fmt.Errorf("%w: site is %s", ErrIllegalState, s.Status)
	}
	if units <= 0 {
		return fmt.Errorf("%w: reservation units", ErrInvalid)
	}
	if s.AvailableUnits < units {
		return ErrCapacity
	}
	return nil
}

type ComplianceReview struct {
	ID          string    `json:"id"`
	SiteID      string    `json:"site_id"`
	ReviewerID  string    `json:"reviewer_id"`
	Decision    string    `json:"decision"`
	EvidenceRef string    `json:"evidence_ref"`
	CreatedAt   time.Time `json:"created_at"`
}
