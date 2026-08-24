package domain

import "errors"

var (
	ErrInvalid       = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrExpired       = errors.New("expired")
	ErrCapacity      = errors.New("insufficient capacity")
	ErrIllegalState  = errors.New("illegal state transition")
	ErrVersion       = errors.New("stale version")
	ErrAlreadyExists = errors.New("already exists")
)
