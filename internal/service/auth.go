package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/requestctx"
	"github.com/11DingKing/asean-ai-launchpad/internal/security"
)

type LoginResult struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      domain.User `json:"user"`
}

func (s *Service) RegisterPartner(ctx context.Context, email, password string) (domain.User, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return domain.User{}, err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("%w: password policy", domain.ErrInvalid)
	}
	id, err := s.IDs.New("usr")
	if err != nil {
		return domain.User{}, err
	}
	now := s.Clock.Now()
	user := domain.User{ID: id, Email: normalized, PasswordHash: hash, Role: domain.RolePartner, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) BootstrapOperator(ctx context.Context, email, password string) (domain.User, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return domain.User{}, err
	}
	existing, findErr := s.Store.FindUserByEmail(ctx, normalized)
	if findErr == nil {
		if existing.Role != domain.RoleOperator {
			return domain.User{}, domain.ErrConflict
		}
		return existing, nil
	}
	if !isNotFound(findErr) {
		return domain.User{}, findErr
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("%w: password policy", domain.ErrInvalid)
	}
	id, err := s.IDs.New("usr")
	if err != nil {
		return domain.User{}, err
	}
	now := s.Clock.Now()
	user := domain.User{ID: id, Email: normalized, PasswordHash: hash, Role: domain.RoleOperator, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return LoginResult{}, domain.ErrUnauthorized
	}
	user, err := s.Store.FindUserByEmail(ctx, normalized)
	if err != nil || !user.Active || !security.VerifyPassword(user.PasswordHash, password) {
		return LoginResult{}, domain.ErrUnauthorized
	}
	plain, hash, err := security.NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	id, err := s.IDs.New("ses")
	if err != nil {
		return LoginResult{}, err
	}
	now := s.Clock.Now()
	session := domain.Session{ID: id, UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(s.SessionTTL), CreatedAt: now, LastSeenAt: now}
	if err := s.Store.CreateSession(ctx, session); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: plain, ExpiresAt: session.ExpiresAt, User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (requestctx.Principal, string, error) {
	if token == "" {
		return requestctx.Principal{}, "", domain.ErrUnauthorized
	}
	session, user, err := s.Store.FindSessionByTokenHash(ctx, security.HashToken(token))
	if err != nil || !user.Active {
		return requestctx.Principal{}, "", domain.ErrUnauthorized
	}
	now := s.Clock.Now()
	if err := session.Usable(now); err != nil {
		return requestctx.Principal{}, "", domain.ErrUnauthorized
	}
	if err := s.Store.TouchSession(ctx, session.ID, now); err != nil {
		return requestctx.Principal{}, "", domain.ErrUnauthorized
	}
	return requestctx.Principal{UserID: user.ID, Role: string(user.Role)}, session.ID, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if _, err := principal(ctx); err != nil {
		return err
	}
	return s.Store.RevokeSession(ctx, sessionID, s.Clock.Now())
}

func (s *Service) CleanupSessions(ctx context.Context) (int64, error) {
	return s.Store.DeleteExpiredSessions(ctx, s.Clock.Now())
}
