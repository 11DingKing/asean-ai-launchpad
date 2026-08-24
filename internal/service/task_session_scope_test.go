package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
)

func TestLogoutRevokesOnlyCurrentSession(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "session-scope.db"))
	if err != nil {
		t.Fatalf("open durable store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	svc := New(database, clock.Fixed{Value: time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)}, &idgen.Sequence{}, 2*time.Hour)
	user, err := svc.RegisterPartner(ctx, "partner.sessions@example.test", "partner-password-123")
	if err != nil {
		t.Fatalf("register partner: %v", err)
	}
	first, err := svc.Login(ctx, user.Email, "partner-password-123")
	if err != nil {
		t.Fatalf("login first device: %v", err)
	}
	second, err := svc.Login(ctx, user.Email, "partner-password-123")
	if err != nil {
		t.Fatalf("login second device: %v", err)
	}
	principal, firstSessionID, err := svc.Authenticate(ctx, first.Token)
	if err != nil {
		t.Fatalf("authenticate first device: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, second.Token); err != nil {
		t.Fatalf("authenticate second device before logout: %v", err)
	}

	logoutCtx := requestctx.WithPrincipal(ctx, principal)
	if err := svc.Logout(logoutCtx, firstSessionID); err != nil {
		t.Fatalf("logout first device: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, first.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("logged out token remains usable: %v", err)
	}
	remaining, _, err := svc.Authenticate(ctx, second.Token)
	if err != nil || remaining.UserID != user.ID {
		t.Fatalf("other device session was revoked: principal=%+v err=%v", remaining, err)
	}
}
