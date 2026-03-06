package service

import "errors"

var (
	ErrInvalidGateKey = errors.New("invalid gate key")
	ErrUsernameTaken  = errors.New("username already in use")
)
