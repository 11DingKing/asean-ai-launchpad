package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func TestStaleWorkerCannotCompleteReclaimedJob(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "lease-fence.db"))
	if err != nil {
		t.Fatalf("open durable store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	job := domain.Job{
		ID: "job_lease_fence", Kind: "activate_deployment", AggregateID: "deployment_lease_fence",
		Payload: `{"deployment_id":"deployment_lease_fence"}`, Status: domain.JobPending,
		MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := insertJob(ctx, database.db, job); err != nil {
		t.Fatalf("seed durable job: %v", err)
	}

	first, err := database.ClaimJob(ctx, "worker-a", now, time.Minute)
	if err != nil {
		t.Fatalf("worker A claim: %v", err)
	}
	reclaimedAt := now.Add(2 * time.Minute)
	second, err := database.ClaimJob(ctx, "worker-b", reclaimedAt, time.Minute)
	if err != nil {
		t.Fatalf("worker B reclaim after lease expiry: %v", err)
	}
	if first.LeaseOwner != "worker-a" || second.LeaseOwner != "worker-b" || second.Attempts != 2 {
		t.Fatalf("unexpected lease sequence: first=%+v second=%+v", first, second)
	}

	if err := database.CompleteJob(ctx, job.ID, "worker-a", reclaimedAt); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale worker completion must conflict, got %v", err)
	}
	if err := database.CompleteJob(ctx, job.ID, "worker-b", reclaimedAt.Add(time.Second)); err != nil {
		t.Fatalf("current lease owner should still complete the job: %v", err)
	}
	if _, err := database.ClaimJob(ctx, "worker-c", reclaimedAt.Add(2*time.Minute), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("successfully completed job became claimable again: %v", err)
	}
}
