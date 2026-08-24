package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
)

type cancellingJobs struct {
	cancel        context.CancelFunc
	job           domain.Job
	completeCalls int
}

func (j *cancellingJobs) ClaimJob(context.Context, string, time.Time, time.Duration) (domain.Job, error) {
	j.job.Status = domain.JobRunning
	j.job.LeaseOwner = "worker-shutting-down"
	j.job.Attempts++
	j.cancel()
	return j.job, nil
}

func (j *cancellingJobs) CompleteJob(ctx context.Context, _ string, _ string, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.completeCalls++
	j.job.Status = domain.JobSucceeded
	return nil
}

func (j *cancellingJobs) RetryJob(ctx context.Context, _ domain.Job, _ string, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.job.Status = domain.JobPending
	return nil
}

func (j *cancellingJobs) CancelJob(context.Context, string, time.Time) error { return nil }

type activationStore struct {
	repository.Store
	deployment       domain.Deployment
	findSawCancelled bool
	transitions      int
}

func (s *activationStore) FindDeployment(ctx context.Context, _ string) (domain.Deployment, error) {
	if err := ctx.Err(); err != nil {
		s.findSawCancelled = true
		return domain.Deployment{}, err
	}
	return s.deployment, nil
}

func (s *activationStore) TransitionDeployment(ctx context.Context, item domain.Deployment, _ domain.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.transitions++
	s.deployment = item
	return nil
}

func TestCancelledActivationWorkerLeavesJobRecoverable(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	jobs := &cancellingJobs{cancel: cancel, job: domain.Job{
		ID: "job_cancelled_activation", Kind: "activate_deployment", AggregateID: "dep_cancelled_activation",
		Payload: `{"deployment_id":"dep_cancelled_activation"}`, Status: domain.JobPending,
		MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}}
	deployments := &activationStore{deployment: domain.Deployment{
		ID: "dep_cancelled_activation", ReservationID: "res_cancelled_activation",
		Status: domain.DeploymentQueued, Version: 1, CreatedAt: now, UpdatedAt: now,
	}}
	handler := service.New(deployments, clock.Fixed{Value: now}, &idgen.Sequence{}, time.Hour)
	worker := Worker{
		Store: jobs, Tasks: handler, Clock: clock.Fixed{Value: now}, Owner: "worker-shutting-down",
		Poll: time.Second, Lease: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	worker.runOne(ctx)

	if !errors.Is(ctx.Err(), context.Canceled) || !deployments.findSawCancelled {
		t.Fatalf("activation path did not observe shutdown cancellation: ctx=%v observed=%v", ctx.Err(), deployments.findSawCancelled)
	}
	if jobs.job.Status == domain.JobSucceeded || jobs.completeCalls != 0 {
		t.Fatalf("cancelled worker acknowledged activation job: status=%s completes=%d", jobs.job.Status, jobs.completeCalls)
	}
	if deployments.deployment.Status != domain.DeploymentQueued || deployments.transitions != 0 {
		t.Fatalf("cancelled activation changed deployment: status=%s transitions=%d", deployments.deployment.Status, deployments.transitions)
	}
}
