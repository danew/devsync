package apperrors

import (
	"errors"
	"fmt"
)

type Kind string

const (
	ErrBranchMismatch         Kind = "branch mismatch"
	ErrHistoryDiverged        Kind = "history diverged"
	ErrHistoryUnknown         Kind = "history unknown"
	ErrMutagenUnavailable     Kind = "mutagen unavailable"
	ErrMutagenUnhealthy       Kind = "mutagen unhealthy"
	ErrRemoteUnreachable      Kind = "remote unreachable"
	ErrRemoteRepoMissing      Kind = "remote repo missing"
	ErrRemoteRepoInvalid      Kind = "remote repo invalid"
	ErrWorkspaceConfigMissing Kind = "workspace config missing"
	ErrWorkspaceLockHeld      Kind = "workspace lock held"
	ErrSessionDrift           Kind = "session drift"
	ErrInitialSyncRisk        Kind = "initial sync risk"
	ErrInterrupted            Kind = "interrupted"
	ErrDetachedHead           Kind = "detached HEAD"
)

type Error struct {
	Kind    Kind
	Message string
	Remedy  string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		if e.Remedy != "" {
			return e.Message + "\nRemediation: " + e.Remedy
		}
		return e.Message
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func New(kind Kind, message string) error {
	return &Error{Kind: kind, Message: message}
}

func NewWithRemedy(kind Kind, message string, remedy string) error {
	return &Error{Kind: kind, Message: message, Remedy: remedy}
}

func Wrap(kind Kind, message string, err error) error {
	return &Error{Kind: kind, Message: message, Err: err}
}

func Is(err error, kind Kind) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind == kind
	}
	return false
}
