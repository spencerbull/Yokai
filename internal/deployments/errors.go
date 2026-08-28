package deployments

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorValidation  ErrorKind = "validation"
	ErrorConflict    ErrorKind = "conflict"
	ErrorDependency  ErrorKind = "dependency"
	ErrorUnavailable ErrorKind = "unavailable"
)

type OperationError struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *OperationError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *OperationError) Unwrap() error { return e.Err }

func WrapError(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	var typed *OperationError
	if errors.As(err, &typed) {
		return err
	}
	return &OperationError{Kind: kind, Op: op, Err: err}
}

func ErrorKindOf(err error) ErrorKind {
	var typed *OperationError
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return ""
}

var (
	ErrIdempotencyConflict = errors.New("idempotency key already used for a different request")
	ErrRecoveryPerformed   = errors.New("incomplete idempotent request was reconciled; use a new idempotency key to create a new deployment")
	ErrContainerNotFound   = errors.New("container not found")
	ErrServiceUnauthorized = errors.New("model service rejected the API key")
)
