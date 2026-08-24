package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func TestRetryAttemptsSurviveUntilPermanentFailure(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "retry-attempts.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open durable store: %v", err)
	}
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	job := domain.Job{ID: "job_retry_persistence", Kind: "activate_deployment", AggregateID: "dep_retry", Payload: `{"deployment_id":"dep_retry"}`, Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := insertJob(ctx, database.db, job); err != nil {
		t.Fatalf("insert durable job: %v", err)
	}

	observed := make([]int, 0, job.MaxAttempts)
	for delivery := 1; delivery <= job.MaxAttempts; delivery++ {
		claimed, err := database.ClaimJob(ctx, fmt.Sprintf("worker_%d", delivery), now, time.Minute)
		if err != nil {
			t.Fatalf("claim delivery %d: %v", delivery, err)
		}
		observed = append(observed, claimed.Attempts)
		if err := database.RetryJob(ctx, claimed, fmt.Sprintf("activation failure %d", delivery), now); err != nil {
			t.Fatalf("retry delivery %d: %v", delivery, err)
		}
		now = now.Add(10 * time.Second)
	}

	var status string
	var attempts int
	if err := database.db.QueryRowContext(ctx, `SELECT status,attempts FROM jobs WHERE id=?`, job.ID).Scan(&status, &attempts); err != nil {
		t.Fatalf("read retry state: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}
	restarted, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen durable store: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	reclaimed, reclaimErr := restarted.ClaimJob(ctx, "worker_after_restart", now.Add(time.Minute), time.Minute)
	if status != string(domain.JobFailed) || attempts != job.MaxAttempts || !errors.Is(reclaimErr, domain.ErrNotFound) {
		t.Fatalf("retry budget was not durable: observed=%v status=%s attempts=%d reclaimed=%+v reclaim_err=%v", observed, status, attempts, reclaimed, reclaimErr)
	}
}
