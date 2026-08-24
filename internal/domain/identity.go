package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

type Role string

const (
	RoleOperator Role = "operator"
	RolePartner  Role = "partner"
)

func (r Role) Valid() bool { return r == RoleOperator || r == RolePartner }

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 {
		return "", fmt.Errorf("%w: email", ErrInvalid)
	}
	return value, nil
}

type Session struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
}

func (s Session) Usable(at time.Time) error {
	if s.RevokedAt != nil {
		return ErrUnauthorized
	}
	if !at.Before(s.ExpiresAt) {
		return ErrExpired
	}
	return nil
}
