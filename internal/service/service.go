package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/audit"
	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/repository"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
)

type Service struct {
	Store      repository.Store
	Clock      clock.Clock
	IDs        idgen.Generator
	Audit      audit.Factory
	SessionTTL time.Duration
}

func New(store repository.Store, clk clock.Clock, ids idgen.Generator, sessionTTL time.Duration) *Service {
	return &Service{Store: store, Clock: clk, IDs: ids, Audit: audit.Factory{IDs: ids}, SessionTTL: sessionTTL}
}

func principal(ctx context.Context) (requestctx.Principal, error) {
	value, ok := requestctx.PrincipalFrom(ctx)
	if !ok || value.UserID == "" {
		return requestctx.Principal{}, domain.ErrUnauthorized
	}
	return value, nil
}

func requireRole(ctx context.Context, roles ...domain.Role) (requestctx.Principal, error) {
	value, err := principal(ctx)
	if err != nil {
		return requestctx.Principal{}, err
	}
	for _, role := range roles {
		if value.Role == string(role) {
			return value, nil
		}
	}
	return requestctx.Principal{}, domain.ErrForbidden
}

func requireText(value, field string, min, max int) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < min || len(value) > max {
		return "", fmt.Errorf("%w: %s", domain.ErrInvalid, field)
	}
	return value, nil
}

func (s *Service) event(ctx context.Context, actorID, action, objectType, objectID, result string, details any) (domain.AuditEvent, error) {
	return s.Audit.Event(actorID, action, objectType, objectID, result, requestctx.RequestID(ctx), details, s.Clock.Now())
}

func (s *Service) job(kind, aggregateID, payload string, available time.Time) (domain.Job, error) {
	id, err := s.IDs.New("job")
	if err != nil {
		return domain.Job{}, err
	}
	now := s.Clock.Now()
	return domain.Job{ID: id, Kind: kind, AggregateID: aggregateID, Payload: payload, Status: domain.JobPending, MaxAttempts: 5, AvailableAt: available.UTC(), CreatedAt: now, UpdatedAt: now}, nil
}
