// Package auth retains the slim authentication helpers the identity service
// needs: console-session validation for the auth middleware and the shared
// password-strength policy used when administrators create or reset users.
package auth

// PasswordValidationError reports invalid password input.
type PasswordValidationError struct {
	Message string
}

func (e PasswordValidationError) Error() string {
	return e.Message
}

// ValidatePasswordStrength validates password strength. Keep in sync with
// services/auth: both enforce the same policy on different write paths.
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return PasswordValidationError{Message: "password must be at least 8 characters"}
	}
	if len(password) > 32 {
		return PasswordValidationError{Message: "password must be no more than 32 characters"}
	}

	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return PasswordValidationError{Message: "password must contain uppercase, lowercase and digit"}
	}

	return nil
}
