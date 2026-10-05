package stackure

import "regexp"

var (
	emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	uuidRegex  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// validateEmail returns a "validation"-coded StackureError when email is not
// a well-formed address.
func validateEmail(email string) error {
	if email == "" {
		return newErr("validation", 0, "email is required")
	}
	if !emailRegex.MatchString(email) {
		return newErr("validation", 0, "invalid email format")
	}
	return nil
}
