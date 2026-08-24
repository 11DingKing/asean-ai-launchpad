package domain

import (
	"errors"
	"testing"
	"time"
)

func validScenario(t *testing.T) Scenario {
	t.Helper()
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	item, err := NewScenario("scn_1", "partner_1", "Cross-border language model", "public services", "restricted", 32, now)
	if err != nil {
		t.Fatalf("new scenario: %v", err)
	}
	return item
}

func TestNewScenarioInitialState(t *testing.T) {
	t.Parallel()
	item := validScenario(t)
	if item.Status != ScenarioSubmitted {
		t.Fatalf("status=%s", item.Status)
	}
	if item.Version != 1 {
		t.Fatalf("version=%d", item.Version)
	}
	if item.RequestedUnits != 32 {
		t.Fatalf("units=%d", item.RequestedUnits)
	}
	if item.DataClassification != "restricted" {
		t.Fatalf("classification=%q", item.DataClassification)
	}
}

func TestNewScenarioValidation(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		name           string
		id             string
		partner        string
		title          string
		sector         string
		classification string
		units          int
	}{
		{name: "blank id", partner: "p", title: "valid title", sector: "energy", classification: "public", units: 1},
		{name: "blank partner", id: "s", title: "valid title", sector: "energy", classification: "public", units: 1},
		{name: "short title", id: "s", partner: "p", title: "x", sector: "energy", classification: "public", units: 1},
		{name: "blank sector", id: "s", partner: "p", title: "valid title", classification: "public", units: 1},
		{name: "unknown classification", id: "s", partner: "p", title: "valid title", sector: "energy", classification: "secret", units: 1},
		{name: "zero units", id: "s", partner: "p", title: "valid title", sector: "energy", classification: "public"},
		{name: "too many units", id: "s", partner: "p", title: "valid title", sector: "energy", classification: "public", units: 100_001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewScenario(tc.id, tc.partner, tc.title, tc.sector, tc.classification, tc.units, now)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid, got %v", err)
			}
		})
	}
}

func TestScenarioStateMachine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from ScenarioStatus
		to   ScenarioStatus
		ok   bool
	}{
		{ScenarioSubmitted, ScenarioReviewing, true},
		{ScenarioSubmitted, ScenarioApproved, false},
		{ScenarioSubmitted, ScenarioRejected, false},
		{ScenarioReviewing, ScenarioApproved, true},
		{ScenarioReviewing, ScenarioRejected, true},
		{ScenarioReviewing, ScenarioSubmitted, false},
		{ScenarioApproved, ScenarioReviewing, false},
		{ScenarioRejected, ScenarioReviewing, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			item := validScenario(t)
			item.Status = tc.from
			before := item.Version
			now := time.Now().UTC()
			err := item.Transition(tc.to, now)
			if tc.ok && err != nil {
				t.Fatalf("transition: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrIllegalState) {
				t.Fatalf("expected illegal state, got %v", err)
			}
			if tc.ok && (item.Status != tc.to || item.Version != before+1 || !item.UpdatedAt.Equal(now)) {
				t.Fatalf("unexpected transition result: %+v", item)
			}
			if !tc.ok && (item.Status != tc.from || item.Version != before) {
				t.Fatalf("illegal transition mutated item: %+v", item)
			}
		})
	}
}
