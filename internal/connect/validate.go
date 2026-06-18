package connect

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
)

// validationError marks an error as coming from client-side input validation
// (malformed value, missing field, wrong shape, etc.) rather than a server-
// side or upstream failure. Handlers should return errors of this type so
// that the connect-go layer maps them to CodeInvalidArgument (HTTP 400)
// instead of the default CodeInternal (HTTP 500).
//
// Wrap with fmt.Errorf("...: %w", err) freely — IsValidationError /
// asConnectError walk the wrap chain with errors.As.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

// NewValidationError formats and returns a validation error. Use this anywhere
// the client sent input we cannot accept; the connect-go handler will surface
// it as CodeInvalidArgument (HTTP 400) with the formatted message as detail.
func NewValidationError(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// IsValidationError reports whether err — or anything in its wrap chain —
// originated from NewValidationError.
func IsValidationError(err error) bool {
	if err == nil {
		return false
	}
	var v *validationError
	return errors.As(err, &v)
}

// asConnectError converts a Go error into the connect-go error envelope,
// choosing the right code based on whether the error is a validation failure
// (CodeInvalidArgument → HTTP 400) or anything else (CodeInternal → HTTP 500).
//
// Use this at the top of every connect-go handler that returns error:
//   return nil, asConnectError(err)
//
// asConnectError is nil-safe: it returns nil if err is nil.
func asConnectError(err error) error {
	if err == nil {
		return nil
	}
	if IsValidationError(err) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}
