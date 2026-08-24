package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
)

type Handler interface {
	ExpireReservation(context.Context, string) error
	ActivateDeploymentJob(context.Context, string) error
	CleanupSessions(context.Context) (int64, error)
}

type Worker struct {
	Store repository.Jobs
	Tasks Handler
	Clock clock.Clock
	Owner string
	Poll  time.Duration
	Lease time.Duration
	Log   *slog.Logger
}

func (w *Worker) Run(ctx context.Context) error {
	if w.Owner == "" || w.Poll <= 0 || w.Lease <= w.Poll {
		return errors.New("invalid worker configuration")
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			w.runOne(ctx)
			timer.Reset(w.Poll)
		}
	}
}

func (w *Worker) runOne(ctx context.Context) {
	job, err := w.Store.ClaimJob(ctx, w.Owner, w.Clock.Now(), w.Lease)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		w.Log.Error("claim durable job", "error", err)
		return
	}
	err = w.handle(ctx, job)
	if err == nil {
		if completeErr := w.Store.CompleteJob(ctx, job.ID, w.Owner, w.Clock.Now()); completeErr != nil {
			w.Log.Error("complete durable job", "job_id", job.ID, "error", completeErr)
		}
		return
	}
	if retryErr := w.Store.RetryJob(ctx, job, err.Error(), w.Clock.Now()); retryErr != nil {
		w.Log.Error("schedule job retry", "job_id", job.ID, "error", retryErr)
		return
	}
	w.Log.Warn("durable job failed", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "error", err)
}

func (w *Worker) handle(ctx context.Context, job domain.Job) error {
	var payload map[string]string
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("decode %s payload: %w", job.Kind, err)
	}
	switch job.Kind {
	case "expire_reservation":
		id := payload["reservation_id"]
		if id == "" {
			id = job.AggregateID
		}
		return w.Tasks.ExpireReservation(ctx, id)
	case "activate_deployment":
		id := payload["deployment_id"]
		if id == "" {
			id = job.AggregateID
		}
		return w.Tasks.ActivateDeploymentJob(ctx, id)
	case "cleanup_sessions":
		_, err := w.Tasks.CleanupSessions(ctx)
		return err
	default:
		return fmt.Errorf("unsupported job kind %q", job.Kind)
	}
}
