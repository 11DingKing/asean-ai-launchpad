package domain

import (
	"fmt"
	"strings"
	"time"
)

type ScenarioStatus string

const (
	ScenarioSubmitted ScenarioStatus = "submitted"
	ScenarioReviewing ScenarioStatus = "reviewing"
	ScenarioApproved  ScenarioStatus = "approved"
	ScenarioRejected  ScenarioStatus = "rejected"
)

type Scenario struct {
	ID                 string         `json:"id"`
	PartnerID          string         `json:"partner_id"`
	Name               string         `json:"name"`
	Sector             string         `json:"sector"`
	RequestedUnits     int            `json:"requested_units"`
	DataClassification string         `json:"data_classification"`
	Status             ScenarioStatus `json:"status"`
	Version            int64          `json:"version"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

func NewScenario(id, partnerID, name, sector, classification string, units int, now time.Time) (Scenario, error) {
	name = strings.TrimSpace(name)
	sector = strings.TrimSpace(sector)
	classification = strings.ToLower(strings.TrimSpace(classification))
	if id == "" || partnerID == "" || len(name) < 3 || len(name) > 160 || sector == "" {
		return Scenario{}, fmt.Errorf("%w: scenario identity", ErrInvalid)
	}
	if units <= 0 || units > 100_000 {
		return Scenario{}, fmt.Errorf("%w: requested units", ErrInvalid)
	}
	switch classification {
	case "public", "restricted", "sovereign":
	default:
		return Scenario{}, fmt.Errorf("%w: data classification", ErrInvalid)
	}
	now = now.UTC()
	return Scenario{ID: id, PartnerID: partnerID, Name: name, Sector: sector, RequestedUnits: units, DataClassification: classification, Status: ScenarioSubmitted, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (s Scenario) CanTransition(next ScenarioStatus) bool {
	switch s.Status {
	case ScenarioSubmitted:
		return next == ScenarioReviewing
	case ScenarioReviewing:
		return next == ScenarioApproved || next == ScenarioRejected
	default:
		return false
	}
}

func (s *Scenario) Transition(next ScenarioStatus, now time.Time) error {
	if !s.CanTransition(next) {
		return fmt.Errorf("%w: scenario %s to %s", ErrIllegalState, s.Status, next)
	}
	s.Status = next
	s.Version++
	s.UpdatedAt = now.UTC()
	return nil
}

type ScenarioReview struct {
	ID         string    `json:"id"`
	ScenarioID string    `json:"scenario_id"`
	ReviewerID string    `json:"reviewer_id"`
	Decision   string    `json:"decision"`
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`
}
