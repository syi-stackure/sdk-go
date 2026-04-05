package stackure

import "regexp"

var (
	emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	uuidRegex  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// validateEmail checks that the given string is a well-formed email address.
func validateEmail(email string) error {
	if email == "" {
		return NewValidationError("Email is required and must be a string")
	}
	if !emailRegex.MatchString(email) {
		return NewValidationError("Invalid email format")
	}
	return nil
}

// validateUUID checks that the given string is a valid UUID v4.
func validateUUID(value string, fieldName string) error {
	if value == "" {
		return NewValidationError(fieldName + " is required and must be a string")
	}
	if !uuidRegex.MatchString(value) {
		return NewValidationError("Invalid " + fieldName + " format (must be a valid UUID)")
	}
	return nil
}
