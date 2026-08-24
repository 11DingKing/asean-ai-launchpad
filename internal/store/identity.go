package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,role,active,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, user.ID, user.Email, user.PasswordHash, user.Role, user.Active, timestamp(user.CreatedAt), timestamp(user.UpdatedAt))
	return classify(err, "create user")
}

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var user domain.User
	var active bool
	var created, updated string
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &active, &created, &updated); err != nil {
		return domain.User{}, err
	}
	var err error
	if user.CreatedAt, err = parseTime(created); err != nil {
		return domain.User{}, err
	}
	if user.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.User{}, err
	}
	user.Active = active
	return user, nil
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	user, err := scanUser(s.db.QueryRowContext(ctx, `SELECT id,email,password_hash,role,active,created_at,updated_at FROM users WHERE email=?`, email))
	return user, classify(err, "find user by email")
}

func (s *Store) FindUserByID(ctx context.Context, id string) (domain.User, error) {
	user, err := scanUser(s.db.QueryRowContext(ctx, `SELECT id,email,password_hash,role,active,created_at,updated_at FROM users WHERE id=?`, id))
	return user, classify(err, "find user by id")
}

func (s *Store) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at,revoked_at,created_at,last_seen_at) VALUES(?,?,?,?,NULL,?,?)`, session.ID, session.UserID, session.TokenHash, timestamp(session.ExpiresAt), timestamp(session.CreatedAt), timestamp(session.LastSeenAt))
	return classify(err, "create session")
}

func scanSessionAndUser(row *sql.Row) (domain.Session, domain.User, error) {
	var session domain.Session
	var user domain.User
	var expires, created, seen, userCreated, userUpdated string
	var revoked sql.NullString
	if err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &expires, &revoked, &created, &seen, &user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Active, &userCreated, &userUpdated); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	var err error
	if session.ExpiresAt, err = parseTime(expires); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if session.CreatedAt, err = parseTime(created); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if session.LastSeenAt, err = parseTime(seen); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if session.RevokedAt, err = optionalTimestamp(revoked); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if user.CreatedAt, err = parseTime(userCreated); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if user.UpdatedAt, err = parseTime(userUpdated); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	return session, user, nil
}

func (s *Store) FindSessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, domain.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.token_hash,s.expires_at,s.revoked_at,s.created_at,s.last_seen_at,u.id,u.email,u.password_hash,u.role,u.active,u.created_at,u.updated_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash)
	session, user, err := scanSessionAndUser(row)
	return session, user, classify(err, "find session")
}

func (s *Store) TouchSession(ctx context.Context, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=? AND revoked_at IS NULL AND expires_at>?`, timestamp(now), id, timestamp(now))
	if err != nil {
		return classify(err, "touch session")
	}
	return requireChanged(result, domain.ErrUnauthorized)
}

func (s *Store) RevokeSession(ctx context.Context, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, timestamp(now), id)
	if err != nil {
		return classify(err, "revoke session")
	}
	return requireChanged(result, domain.ErrUnauthorized)
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, timestamp(now), userID)
	if err != nil {
		return classify(err, "revoke user sessions")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count revoked user sessions: %w", err)
	}
	if count == 0 {
		return domain.ErrUnauthorized
	}
	return nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=? OR (revoked_at IS NOT NULL AND revoked_at<=?)`, timestamp(now), timestamp(now.Add(-24*time.Hour)))
	if err != nil {
		return 0, classify(err, "delete expired sessions")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted sessions: %w", err)
	}
	return count, nil
}
