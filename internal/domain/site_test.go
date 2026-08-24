package domain

import (
	"errors"
	"testing"
	"time"
)

func validSite(t *testing.T) ComputeSite {
	t.Helper()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	site, err := NewComputeSite("site_1", "usr_operator", "Jakarta GPU Corridor", "id", "Asia/Jakarta", 128, now)
	if err != nil {
		t.Fatalf("new site: %v", err)
	}
	return site
}

func TestNewComputeSiteNormalizesAndInitializes(t *testing.T) {
	t.Parallel()
	site := validSite(t)
	if site.CountryCode != "ID" {
		t.Fatalf("country=%q", site.CountryCode)
	}
	if site.Status != SiteDraft {
		t.Fatalf("status=%q", site.Status)
	}
	if site.AvailableUnits != 128 || site.TotalUnits != 128 {
		t.Fatalf("capacity=%d/%d", site.AvailableUnits, site.TotalUnits)
	}
	if site.Version != 1 {
		t.Fatalf("version=%d", site.Version)
	}
}

func TestNewComputeSiteRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		name     string
		id       string
		operator string
		siteName string
		country  string
		zone     string
		units    int
	}{
		{name: "missing id", operator: "op", siteName: "Valid Site", country: "SG", zone: "Asia/Singapore", units: 1},
		{name: "missing owner", id: "site", siteName: "Valid Site", country: "SG", zone: "Asia/Singapore", units: 1},
		{name: "short name", id: "site", operator: "op", siteName: "x", country: "SG", zone: "Asia/Singapore", units: 1},
		{name: "bad country", id: "site", operator: "op", siteName: "Valid Site", country: "SEA", zone: "Asia/Singapore", units: 1},
		{name: "unknown zone", id: "site", operator: "op", siteName: "Valid Site", country: "SG", zone: "Moon/Base", units: 1},
		{name: "zero units", id: "site", operator: "op", siteName: "Valid Site", country: "SG", zone: "Asia/Singapore"},
		{name: "excess units", id: "site", operator: "op", siteName: "Valid Site", country: "SG", zone: "Asia/Singapore", units: 1_000_001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewComputeSite(tc.id, tc.operator, tc.siteName, tc.country, tc.zone, tc.units, now)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid error, got %v", err)
			}
		})
	}
}

func TestSiteStateMachine(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	cases := []struct {
		from SiteStatus
		to   SiteStatus
		ok   bool
	}{
		{SiteDraft, SiteVerified, true},
		{SiteDraft, SiteSuspended, true},
		{SiteDraft, SiteActive, false},
		{SiteVerified, SiteActive, true},
		{SiteVerified, SiteSuspended, true},
		{SiteVerified, SiteDraft, false},
		{SiteActive, SiteSuspended, true},
		{SiteActive, SiteVerified, false},
		{SiteSuspended, SiteActive, true},
		{SiteSuspended, SiteDraft, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			site := validSite(t)
			site.Status = tc.from
			before := site.Version
			err := site.Transition(tc.to, now)
			if tc.ok {
				if err != nil {
					t.Fatalf("transition: %v", err)
				}
				if site.Status != tc.to || site.Version != before+1 || !site.UpdatedAt.Equal(now) {
					t.Fatalf("unexpected transitioned site: %+v", site)
				}
				return
			}
			if !errors.Is(err, ErrIllegalState) {
				t.Fatalf("expected illegal state, got %v", err)
			}
			if site.Status != tc.from || site.Version != before {
				t.Fatalf("illegal transition mutated site: %+v", site)
			}
		})
	}
}

func TestSiteCapacityRules(t *testing.T) {
	t.Parallel()
	site := validSite(t)
	if err := site.CanReserve(1); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("draft should reject reservation: %v", err)
	}
	site.Status = SiteActive
	if err := site.CanReserve(0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero units should be invalid: %v", err)
	}
	if err := site.CanReserve(129); !errors.Is(err, ErrCapacity) {
		t.Fatalf("oversized request should fail capacity: %v", err)
	}
	if err := site.CanReserve(128); err != nil {
		t.Fatalf("full available capacity should be reservable: %v", err)
	}
	site.AvailableUnits = 0
	if err := site.CanReserve(1); !errors.Is(err, ErrCapacity) {
		t.Fatalf("empty site should fail capacity: %v", err)
	}
}
