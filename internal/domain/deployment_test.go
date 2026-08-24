package domain

import (
	"errors"
	"testing"
	"time"
)

func validReservationForDeployment(t *testing.T) (Reservation, time.Time) {
	t.Helper()
	scenario, site, now := approvedScenarioAndSite(t)
	item, err := NewReservation("rsv_1", scenario, site, scenario.PartnerID, time.Hour, now)
	if err != nil {
		t.Fatalf("reservation: %v", err)
	}
	return item, now
}

func TestNewDeploymentRequiresDeployableReservation(t *testing.T) {
	t.Parallel()
	reservation, now := validReservationForDeployment(t)
	item, err := NewDeployment("dep_1", reservation, now)
	if err != nil {
		t.Fatalf("new deployment: %v", err)
	}
	if item.Status != DeploymentQueued || item.Version != 1 {
		t.Fatalf("unexpected deployment: %+v", item)
	}
	if item.ReservationID != reservation.ID {
		t.Fatalf("reservation=%q", item.ReservationID)
	}
	if _, err := NewDeployment("", reservation, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank id: %v", err)
	}
	reservation.Status = ReservationReleased
	if _, err := NewDeployment("dep_2", reservation, now); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("released reservation: %v", err)
	}
}

func TestDeploymentStateMachine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from DeploymentStatus
		to   DeploymentStatus
		ok   bool
	}{
		{DeploymentQueued, DeploymentActivating, true},
		{DeploymentQueued, DeploymentFailed, true},
		{DeploymentQueued, DeploymentRunning, false},
		{DeploymentActivating, DeploymentRunning, true},
		{DeploymentActivating, DeploymentFailed, true},
		{DeploymentRunning, DeploymentStopping, true},
		{DeploymentRunning, DeploymentFailed, true},
		{DeploymentStopping, DeploymentStopped, true},
		{DeploymentStopping, DeploymentFailed, true},
		{DeploymentStopped, DeploymentRunning, false},
		{DeploymentFailed, DeploymentQueued, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			item := Deployment{Status: tc.from, Version: 5}
			now := time.Now().UTC()
			err := item.Transition(tc.to, "https://runtime.example/endpoint", now)
			if tc.ok && err != nil {
				t.Fatalf("transition: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrIllegalState) {
				t.Fatalf("expected illegal state, got %v", err)
			}
			if !tc.ok {
				return
			}
			if item.Version != 6 || item.Status != tc.to {
				t.Fatalf("unexpected item: %+v", item)
			}
			if tc.to == DeploymentRunning && (item.StartedAt == nil || item.Endpoint == "") {
				t.Fatalf("running metadata missing: %+v", item)
			}
			if tc.to == DeploymentStopped && item.StoppedAt == nil {
				t.Fatalf("stopped timestamp missing: %+v", item)
			}
		})
	}
}

func TestNewUsageRecordValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	start := now.Add(-time.Hour)
	end := now
	item, err := NewUsageRecord("use_1", "dep_1", "partner_1", start, end, 3600, now)
	if err != nil {
		t.Fatalf("new usage: %v", err)
	}
	if item.UnitSeconds != 3600 || !item.PeriodStart.Equal(start) || !item.PeriodEnd.Equal(end) {
		t.Fatalf("unexpected usage: %+v", item)
	}
	cases := []struct {
		name       string
		id         string
		deployment string
		partner    string
		start      time.Time
		end        time.Time
		units      int64
	}{
		{name: "blank id", deployment: "d", partner: "p", start: start, end: end, units: 1},
		{name: "blank deployment", id: "u", partner: "p", start: start, end: end, units: 1},
		{name: "blank partner", id: "u", deployment: "d", start: start, end: end, units: 1},
		{name: "reversed range", id: "u", deployment: "d", partner: "p", start: end, end: start, units: 1},
		{name: "equal range", id: "u", deployment: "d", partner: "p", start: start, end: start, units: 1},
		{name: "negative usage", id: "u", deployment: "d", partner: "p", start: start, end: end, units: -1},
		{name: "period too long", id: "u", deployment: "d", partner: "p", start: start.Add(-32 * 24 * time.Hour), end: end, units: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewUsageRecord(tc.id, tc.deployment, tc.partner, tc.start, tc.end, tc.units, now)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid, got %v", err)
			}
		})
	}
}
