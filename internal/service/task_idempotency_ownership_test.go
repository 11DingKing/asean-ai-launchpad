package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
)

func TestInFlightIdempotencyClaimCannotBeStolen(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "idempotency-ownership.db"))
	if err != nil {
		t.Fatalf("open durable store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	svc := New(database, clock.Fixed{Value: now}, &idgen.Sequence{}, time.Hour)
	body := []byte(`{"reservation_id":"rsv_active"}`)

	first, firstClaimed, err := svc.ClaimIdempotency(ctx, "partner_1", "POST", "/v1/deployments", "create-deployment-1", body)
	if err != nil || !firstClaimed {
		t.Fatalf("first request did not own claim: claimed=%v err=%v", firstClaimed, err)
	}
	second, secondClaimed, err := svc.ClaimIdempotency(ctx, "partner_1", "POST", "/v1/deployments", "create-deployment-1", body)
	if err != nil {
		t.Fatalf("inspect in-flight claim: %v", err)
	}
	if secondClaimed || second.Scope != first.Scope || second.ResponseStatus != 0 {
		t.Fatalf("second request stole in-flight claim: claimed=%v first=%+v second=%+v", secondClaimed, first, second)
	}

	response := `{"deployment":{"id":"dep_once"}}`
	if err := svc.CompleteIdempotency(ctx, first.Scope, 201, response); err != nil {
		t.Fatalf("complete first response: %v", err)
	}
	replayed, claimedAgain, err := svc.ClaimIdempotency(ctx, "partner_1", "POST", "/v1/deployments", "create-deployment-1", body)
	if err != nil {
		t.Fatalf("replay completed response: %v", err)
	}
	if claimedAgain || replayed.ResponseStatus != 201 || replayed.ResponseBody != response {
		t.Fatalf("completed response was not replayed: claimed=%v record=%+v", claimedAgain, replayed)
	}
}
