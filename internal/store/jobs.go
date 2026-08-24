package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func insertJob(ctx context.Context, q querier, job domain.Job) error {
	_, err := q.ExecContext(ctx, `INSERT INTO jobs(id,kind,aggregate_id,payload,status,attempts,max_attempts,available_at,lease_owner,lease_until,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,NULL,NULL,'',?,?)`, job.ID, job.Kind, job.AggregateID, job.Payload, job.Status, job.Attempts, job.MaxAttempts, timestamp(job.AvailableAt), timestamp(job.CreatedAt), timestamp(job.UpdatedAt))
	return classify(err, "insert job")
}

func scanJob(row interface{ Scan(...any) error }) (domain.Job, error) {
	var item domain.Job
	var available, created, updated string
	var leaseOwner, leaseUntil sql.NullString
	if err := row.Scan(&item.ID, &item.Kind, &item.AggregateID, &item.Payload, &item.Status, &item.Attempts, &item.MaxAttempts, &available, &leaseOwner, &leaseUntil, &item.LastError, &created, &updated); err != nil {
		return domain.Job{}, err
	}
	item.LeaseOwner = leaseOwner.String
	var err error
	if item.AvailableAt, err = parseTime(available); err != nil {
		return domain.Job{}, err
	}
	if item.LeaseUntil, err = optionalTimestamp(leaseUntil); err != nil {
		return domain.Job{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.Job{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.Job{}, err
	}
	return item, nil
}

func (s *Store) ClaimJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (domain.Job, error) {
	var claimed domain.Job
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT id,kind,aggregate_id,payload,status,attempts,max_attempts,available_at,lease_owner,lease_until,last_error,created_at,updated_at FROM jobs WHERE status IN ('pending','running') AND available_at<=? AND (status='pending' OR lease_until IS NULL OR lease_until<=?) ORDER BY available_at,id LIMIT 1`, timestamp(now), timestamp(now))
		item, err := scanJob(row)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return classify(err, "select claimable job")
		}
		until := now.Add(lease)
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='running',attempts=attempts+1,lease_owner=?,lease_until=?,updated_at=? WHERE id=? AND (status='pending' OR lease_until IS NULL OR lease_until<=?)`, owner, timestamp(until), timestamp(now), item.ID, timestamp(now))
		if err != nil {
			return classify(err, "claim job")
		}
		if err := requireChanged(result, domain.ErrConflict); err != nil {
			return err
		}
		item.Status = domain.JobRunning
		item.Attempts++
		item.LeaseOwner = owner
		item.LeaseUntil = &until
		item.UpdatedAt = now.UTC()
		claimed = item
		return nil
	})
	return claimed, err
}

func (s *Store) CompleteJob(ctx context.Context, id, owner string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='succeeded',lease_owner=NULL,lease_until=NULL,last_error='',updated_at=? WHERE id=? AND status='running' AND lease_owner=?`, timestamp(now), id, owner)
	if err != nil {
		return classify(err, "complete job")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count completed job rows: %w", err)
	}
	if changed == 1 {
		return nil
	}
	fallback, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='succeeded',lease_owner=NULL,lease_until=NULL,last_error='',updated_at=? WHERE id=? AND status='running'`, timestamp(now), id)
	if err != nil {
		return classify(err, "complete reclaimed job")
	}
	return requireChanged(fallback, domain.ErrConflict)
}

func (s *Store) RetryJob(ctx context.Context, job domain.Job, message string, now time.Time) error {
	status := domain.JobPending
	available := now.Add(backoff(job.Attempts))
	if job.Attempts >= job.MaxAttempts {
		status = domain.JobFailed
		available = now
	}
	result, err := s.db.ExecContext(ctx, `UPDATE jobs SET status=?,available_at=?,lease_owner=NULL,lease_until=NULL,last_error=?,updated_at=? WHERE id=? AND status='running' AND lease_owner=?`, status, timestamp(available), message, timestamp(now), job.ID, job.LeaseOwner)
	if err != nil {
		return classify(err, "retry job")
	}
	return requireChanged(result, domain.ErrConflict)
}

func (s *Store) CancelJob(ctx context.Context, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='cancelled',lease_owner=NULL,lease_until=NULL,updated_at=? WHERE id=? AND status IN ('pending','running')`, timestamp(now), id)
	if err != nil {
		return classify(err, "cancel job")
	}
	return requireChanged(result, domain.ErrConflict)
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}
