package stackure

import "fmt"

// StackureError is the single error type returned by every SDK function.
//
// Callers can type-switch once and branch on Code for different categories:
//
//	var se *stackure.StackureError
//	if errors.As(err, &se) {
//	    switch se.Code {
//	    case "validation": // bad input
//	    case "auth":       // 401 from the API
//	    case "forbidden":  // 403 from the API
//	    case "timeout":    // request exceeded timeout
//	    case "network":    // everything else
//	    }
//	}
type StackureError struct {
	// Code identifies the error category. One of:
	// "validation", "auth", "forbidden", "timeout", "network".
	Code string
	// StatusCode is the HTTP status returned by the API, or 0 if the
	// error happened before a response was received.
	StatusCode int
	// Message is a human-readable description.
	Message string
}

// Error implements the error interface.
func (e *StackureError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("stackure: %s (status %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("stackure: %s: %s", e.Code, e.Message)
}

// newErr is the internal constructor used throughout the package.
func newErr(code string, statusCode int, message string) *StackureError {
	return &StackureError{Code: code, StatusCode: statusCode, Message: message}
}
