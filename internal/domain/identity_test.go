package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
		err   bool
	}{
		{name: "lowercases and trims", input: "  Partner@Example.COM ", want: "partner@example.com"},
		{name: "keeps plus address", input: "ops+asean@example.test", want: "ops+asean@example.test"},
		{name: "rejects missing domain", input: "partner", err: true},
		{name: "rejects display name", input: "Partner <partner@example.com>", err: true},
		{name: "rejects blank", input: " ", err: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeEmail(tc.input)
			if tc.err {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("expected invalid error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize email: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRoleValidity(t *testing.T) {
	t.Parallel()
	if !RoleOperator.Valid() {
		t.Fatal("operator should be valid")
	}
	if !RolePartner.Valid() {
		t.Fatal("partner should be valid")
	}
	if Role("administrator").Valid() {
		t.Fatal("unexpected administrator role")
	}
	if Role("").Valid() {
		t.Fatal("blank role should be invalid")
	}
}

func TestSessionUsableLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	session := Session{ID: "ses_1", UserID: "usr_1", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastSeenAt: now}
	if err := session.Usable(now); err != nil {
		t.Fatalf("new session should be usable: %v", err)
	}
	if err := session.Usable(now.Add(59 * time.Minute)); err != nil {
		t.Fatalf("session before expiry should be usable: %v", err)
	}
	if err := session.Usable(now.Add(time.Hour)); !errors.Is(err, ErrExpired) {
		t.Fatalf("exact expiry should be expired, got %v", err)
	}
	if err := session.Usable(now.Add(2 * time.Hour)); !errors.Is(err, ErrExpired) {
		t.Fatalf("past expiry should be expired, got %v", err)
	}
	revoked := now.Add(10 * time.Minute)
	session.RevokedAt = &revoked
	if err := session.Usable(now.Add(11 * time.Minute)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked session should be unauthorized, got %v", err)
	}
}

func TestPageNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input Page
		want  Page
	}{
		{input: Page{}, want: Page{Limit: 20}},
		{input: Page{Limit: -1, Offset: -2}, want: Page{Limit: 20}},
		{input: Page{Limit: 200, Offset: 4}, want: Page{Limit: 100, Offset: 4}},
		{input: Page{Limit: 25, Offset: 50}, want: Page{Limit: 25, Offset: 50}},
	}
	for _, tc := range cases {
		if got := tc.input.Normalize(); got != tc.want {
			t.Errorf("Normalize(%+v)=%+v, want %+v", tc.input, got, tc.want)
		}
	}
}
