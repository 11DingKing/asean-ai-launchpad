package domain

import (
	"errors"
	"testing"
	"time"
)

func approvedScenarioAndSite(t *testing.T) (Scenario, ComputeSite, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 24, 8, 30, 0, 0, time.UTC)
	scenario := validScenario(t)
	scenario.Status = ScenarioApproved
	site := validSite(t)
	site.Status = SiteActive
	return scenario, site, now
}

func TestNewReservationUsesScenarioCapacityAndExpiry(t *testing.T) {
	t.Parallel()
	scenario, site, now := approvedScenarioAndSite(t)
	item, err := NewReservation("rsv_1", scenario, site, scenario.PartnerID, 90*time.Minute, now)
	if err != nil {
		t.Fatalf("new reservation: %v", err)
	}
	if item.Units != scenario.RequestedUnits {
		t.Fatalf("units=%d want=%d", item.Units, scenario.RequestedUnits)
	}
	if item.Status != ReservationConfirmed {
		t.Fatalf("status=%s", item.Status)
	}
	if !item.ExpiresAt.Equal(now.Add(90 * time.Minute)) {
		t.Fatalf("expires=%s", item.ExpiresAt)
	}
	if item.Version != 1 {
		t.Fatalf("version=%d", item.Version)
	}
}

func TestNewReservationRejectsBrokenPrerequisites(t *testing.T) {
	t.Parallel()
	scenario, site, now := approvedScenarioAndSite(t)
	cases := []struct {
		name      string
		id        string
		owner     string
		mutate    func(*Scenario, *ComputeSite)
		ttl       time.Duration
		wantError error
	}{
		{name: "blank id", owner: scenario.PartnerID, ttl: time.Hour, wantError: ErrForbidden},
		{name: "wrong owner", id: "r", owner: "other", ttl: time.Hour, wantError: ErrForbidden},
		{name: "unapproved scenario", id: "r", owner: scenario.PartnerID, ttl: time.Hour, mutate: func(s *Scenario, _ *ComputeSite) { s.Status = ScenarioReviewing }, wantError: ErrIllegalState},
		{name: "inactive site", id: "r", owner: scenario.PartnerID, ttl: time.Hour, mutate: func(_ *Scenario, s *ComputeSite) { s.Status = SiteSuspended }, wantError: ErrIllegalState},
		{name: "insufficient site capacity", id: "r", owner: scenario.PartnerID, ttl: time.Hour, mutate: func(_ *Scenario, s *ComputeSite) { s.AvailableUnits = 1 }, wantError: ErrCapacity},
		{name: "short ttl", id: "r", owner: scenario.PartnerID, ttl: time.Second, wantError: ErrInvalid},
		{name: "long ttl", id: "r", owner: scenario.PartnerID, ttl: 8 * 24 * time.Hour, wantError: ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scenarioCopy := scenario
			siteCopy := site
			if tc.mutate != nil {
				tc.mutate(&scenarioCopy, &siteCopy)
			}
			_, err := NewReservation(tc.id, scenarioCopy, siteCopy, tc.owner, tc.ttl, now)
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("got %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestReservationStateMachine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from ReservationStatus
		to   ReservationStatus
		ok   bool
	}{
		{ReservationPending, ReservationConfirmed, true},
		{ReservationPending, ReservationReleased, true},
		{ReservationPending, ReservationExpired, true},
		{ReservationConfirmed, ReservationReleased, true},
		{ReservationConfirmed, ReservationExpired, true},
		{ReservationConfirmed, ReservationPending, false},
		{ReservationReleased, ReservationConfirmed, false},
		{ReservationExpired, ReservationReleased, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			item := Reservation{Status: tc.from, Version: 3}
			err := item.Transition(tc.to, time.Now())
			if tc.ok && err != nil {
				t.Fatalf("transition: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrIllegalState) {
				t.Fatalf("expected illegal state, got %v", err)
			}
			if tc.ok && item.Version != 4 {
				t.Fatalf("version=%d", item.Version)
			}
		})
	}
}

func TestReservationDeployableBoundary(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	item := Reservation{Status: ReservationConfirmed, ExpiresAt: now.Add(time.Minute)}
	if err := item.Deployable(now); err != nil {
		t.Fatalf("confirmed before expiry should deploy: %v", err)
	}
	if err := item.Deployable(item.ExpiresAt); !errors.Is(err, ErrExpired) {
		t.Fatalf("exact expiry should fail: %v", err)
	}
	item.Status = ReservationReleased
	if err := item.Deployable(now); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("released reservation should fail state: %v", err)
	}
}
