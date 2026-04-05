package stackure

import "fmt"

// StackureError is the base error type for all Stackure SDK errors.
type StackureError struct {
	Message    string
	Code       string
	StatusCode int
}

func (e *StackureError) Error() string {
	return fmt.Sprintf("StackureError [%s] (status %d): %s", e.Code, e.StatusCode, e.Message)
}

// ValidationError is returned when input validation fails before a request is made.
type ValidationError struct {
	StackureError
}

// NewValidationError creates a new ValidationError.
func NewValidationError(message string) *ValidationError {
	return &ValidationError{
		StackureError: StackureError{
			Message:    message,
			Code:       "VALIDATION_ERROR",
			StatusCode: 400,
		},
	}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("ValidationError: %s", e.Message)
}

// NetworkError is returned when an HTTP request fails or the API returns an error response.
type NetworkError struct {
	StackureError
}

// NewNetworkError creates a new NetworkError.
func NewNetworkError(message string, statusCode int) *NetworkError {
	return &NetworkError{
		StackureError: StackureError{
			Message:    message,
			Code:       "NETWORK_ERROR",
			StatusCode: statusCode,
		},
	}
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("NetworkError (status %d): %s", e.StatusCode, e.Message)
}

// AuthenticationError is returned when the API returns a 401 Unauthorized response.
type AuthenticationError struct {
	StackureError
}

// NewAuthenticationError creates a new AuthenticationError.
func NewAuthenticationError(message string) *AuthenticationError {
	return &AuthenticationError{
		StackureError: StackureError{
			Message:    message,
			Code:       "AUTHENTICATION_ERROR",
			StatusCode: 401,
		},
	}
}

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("AuthenticationError: %s", e.Message)
}

// TimeoutError is returned when an HTTP request exceeds the configured timeout.
type TimeoutError struct {
	StackureError
}

// NewTimeoutError creates a new TimeoutError.
func NewTimeoutError(message string) *TimeoutError {
	if message == "" {
		message = "Request timed out"
	}
	return &TimeoutError{
		StackureError: StackureError{
			Message:    message,
			Code:       "TIMEOUT_ERROR",
			StatusCode: 408,
		},
	}
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("TimeoutError: %s", e.Message)
}

// ForbiddenError is returned when the authenticated user lacks the required role or permission.
type ForbiddenError struct {
	StackureError
}

// NewForbiddenError creates a new ForbiddenError.
func NewForbiddenError(message string) *ForbiddenError {
	return &ForbiddenError{
		StackureError: StackureError{
			Message:    message,
			Code:       "FORBIDDEN_ERROR",
			StatusCode: 403,
		},
	}
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("ForbiddenError: %s", e.Message)
}
