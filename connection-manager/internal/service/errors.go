// Package service provides common errors for services.
package service

import "errors"

// Service errors.
var (
	ErrInvalidToken    = errors.New("invalid or expired installation token")
	ErrExpiredToken    = errors.New("installation token has expired")
	ErrDuplicateAgent  = errors.New("agent with this hostname already exists")
	ErrAgentNotFound   = errors.New("agent not found")
	ErrCertNotFound    = errors.New("certificate not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidCSR      = errors.New("invalid certificate signing request")
	ErrInvalidPassword = errors.New("invalid password")
	ErrAccountLocked   = errors.New("account is locked")
	ErrInternal        = errors.New("internal service error")
)
