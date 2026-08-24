package store

import (
	"context"
	"errors"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) ClaimIdempotency(ctx context.Context, item domain.IdempotencyRecord) (domain.IdempotencyRecord, bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO idempotency_records(scope,actor_id,method,path,request_hash,response_status,response_body,expires_at,created_at) VALUES(?,?,?,?,?,0,'',?,?) ON CONFLICT(scope) DO NOTHING`, item.Scope, item.ActorID, item.Method, item.Path, item.RequestHash, timestamp(item.ExpiresAt), timestamp(item.CreatedAt))
	if err != nil {
		return domain.IdempotencyRecord{}, false, classify(err, "claim idempotency key")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return domain.IdempotencyRecord{}, false, err
	}
	if changed == 1 {
		return item, true, nil
	}
	existing, err := scanIdempotency(s.db.QueryRowContext(ctx, `SELECT scope,actor_id,method,path,request_hash,response_status,response_body,expires_at,created_at FROM idempotency_records WHERE scope=?`, item.Scope))
	if err != nil {
		return domain.IdempotencyRecord{}, false, classify(err, "read idempotency key")
	}
	if !item.CreatedAt.Before(existing.ExpiresAt) {
		deleted, err := s.db.ExecContext(ctx, `DELETE FROM idempotency_records WHERE scope=? AND expires_at<=?`, item.Scope, timestamp(item.CreatedAt))
		if err != nil {
			return domain.IdempotencyRecord{}, false, classify(err, "expire idempotency key")
		}
		count, _ := deleted.RowsAffected()
		if count == 1 {
			return s.ClaimIdempotency(ctx, item)
		}
	}
	return existing, false, nil
}

func scanIdempotency(row interface{ Scan(...any) error }) (domain.IdempotencyRecord, error) {
	var item domain.IdempotencyRecord
	var expires, created string
	if err := row.Scan(&item.Scope, &item.ActorID, &item.Method, &item.Path, &item.RequestHash, &item.ResponseStatus, &item.ResponseBody, &expires, &created); err != nil {
		return domain.IdempotencyRecord{}, err
	}
	var err error
	if item.ExpiresAt, err = parseTime(expires); err != nil {
		return domain.IdempotencyRecord{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return domain.IdempotencyRecord{}, err
	}
	return item, nil
}

func (s *Store) CompleteIdempotency(ctx context.Context, scope string, status int, body string) error {
	if status < 200 || status >= 300 {
		return domain.ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE idempotency_records SET response_status=?,response_body=? WHERE scope=? AND response_status=0`, status, body, scope)
	if err != nil {
		return classify(err, "complete idempotency key")
	}
	return requireChanged(result, domain.ErrConflict)
}

func (s *Store) AbandonIdempotency(ctx context.Context, scope string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM idempotency_records WHERE scope=? AND response_status=0`, scope)
	if err != nil {
		return classify(err, "abandon idempotency key")
	}
	if err := requireChanged(result, domain.ErrConflict); err != nil && !errors.Is(err, domain.ErrConflict) {
		return err
	}
	return nil
}
