package service

import (
	"context"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idempotency"
)

func (s *Service) ClaimIdempotency(ctx context.Context, actorID, method, path, key string, body []byte) (domain.IdempotencyRecord, bool, error) {
	scope, err := idempotency.Scope(actorID, method, path, key)
	if err != nil {
		return domain.IdempotencyRecord{}, false, domain.ErrInvalid
	}
	now := s.Clock.Now()
	item := domain.IdempotencyRecord{Scope: scope, ActorID: actorID, Method: method, Path: path, RequestHash: idempotency.RequestHash(body), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
	existing, claimed, err := s.Store.ClaimIdempotency(ctx, item)
	if err != nil {
		return domain.IdempotencyRecord{}, false, err
	}
	if !claimed && existing.ResponseStatus == 0 {
		return s.Store.ReclaimIdempotency(ctx, item)
	}
	if !claimed && existing.RequestHash != item.RequestHash {
		return domain.IdempotencyRecord{}, false, domain.ErrConflict
	}
	return existing, claimed, nil
}

func (s *Service) CompleteIdempotency(ctx context.Context, scope string, status int, body string) error {
	return s.Store.CompleteIdempotency(ctx, scope, status, body)
}

func (s *Service) AbandonIdempotency(ctx context.Context, scope string) error {
	return s.Store.AbandonIdempotency(ctx, scope)
}
