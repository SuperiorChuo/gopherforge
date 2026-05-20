package auth

import (
	"errors"
	"testing"
)

func TestValidateProfileEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{name: "empty", email: "", wantErr: false},
		{name: "valid", email: "user@example.com", wantErr: false},
		{name: "missing at", email: "user.example.com", wantErr: true},
		{name: "missing domain dot", email: "user@example", wantErr: true},
		{name: "empty domain label", email: "user@.example.com", wantErr: true},
		{name: "contains space", email: "user name@example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfileEmail(tt.email)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr {
				var validationErr ProfileValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected ProfileValidationError, got %T", err)
				}
			}
		})
	}
}

func TestValidateProfilePhone(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{name: "empty", phone: "", wantErr: false},
		{name: "mobile", phone: "13800138000", wantErr: false},
		{name: "international", phone: "+86 138-0013-8000", wantErr: false},
		{name: "letters", phone: "13800abc000", wantErr: true},
		{name: "too short", phone: "1234", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfilePhone(tt.phone)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr {
				var validationErr ProfileValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected ProfileValidationError, got %T", err)
				}
			}
		})
	}
}

func TestValidatePasswordStrengthReturnsPasswordValidationError(t *testing.T) {
	err := validatePasswordStrength("short")
	if err == nil {
		t.Fatal("expected error")
	}
	var validationErr PasswordValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected PasswordValidationError, got %T", err)
	}
}
