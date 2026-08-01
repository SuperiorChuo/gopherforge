package mask

import (
	"strings"
	"testing"
	"time"
)

func TestIsSensitiveFieldSupportsSettingsAndCamelCaseNames(t *testing.T) {
	for _, field := range []string{
		"api_key",
		"apiKey",
		"accessKeySecret",
		"smtp-password",
		"nested.private_key",
		"password_hash",
		"credentialPasswordHash",
		"recovery_code",
		"mfaRecoveryCodes",
		"backup_code",
		"totpBackupCodes",
	} {
		if !IsSensitiveField(field) {
			t.Errorf("IsSensitiveField(%q) = false, want true", field)
		}
	}
	for _, field := range []string{
		"setting_key",
		"public_key",
		"monkey",
		"username",
		"must_change_password",
		"mustChangePassword",
	} {
		if IsSensitiveField(field) {
			t.Errorf("IsSensitiveField(%q) = true, want false", field)
		}
	}
}

func TestRedactJSONMasksNestedSecretsAndFailsClosed(t *testing.T) {
	body := `{"api_key":"top-secret","nested":{"accessKeySecret":"also-secret"},"name":"provider"}`
	got := RedactJSON(body)
	if strings.Contains(got, "top-secret") || strings.Contains(got, "also-secret") {
		t.Fatalf("RedactJSON() leaked a secret: %s", got)
	}
	if !strings.Contains(got, `"api_key":"********"`) || !strings.Contains(got, `"accessKeySecret":"********"`) {
		t.Fatalf("RedactJSON() did not mask expected fields: %s", got)
	}

	if got := RedactJSON(`{"api_key":"secret"...[truncated]`); got != InvalidJSONRedactedValue {
		t.Fatalf("RedactJSON(invalid) = %q, want fail-closed marker", got)
	}
}

func TestRestoreRedactedSensitiveValuesPreservesStoredSecrets(t *testing.T) {
	stored := map[string]any{
		"api_key": "stored-secret",
		"nested":  map[string]any{"password": "stored-password", "enabled": true},
	}
	incoming := map[string]any{
		"api_key":    RedactedValue,
		"nested":     map[string]any{"password": "***", "enabled": false},
		"new_secret": RedactedValue,
	}

	got, ok := RestoreRedactedSensitiveValues(incoming, stored).(map[string]any)
	if !ok {
		t.Fatalf("RestoreRedactedSensitiveValues() type = %T, want map", got)
	}
	if got["api_key"] != "stored-secret" {
		t.Fatalf("api_key = %v, want stored value", got["api_key"])
	}
	nested := got["nested"].(map[string]any)
	if nested["password"] != "stored-password" || nested["enabled"] != false {
		t.Fatalf("nested value = %#v, want secret preserved and regular value updated", nested)
	}
	if _, exists := got["new_secret"]; exists {
		t.Fatalf("placeholder without stored value must be omitted: %#v", got)
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		maskType string
		input    string
		want     string
	}{
		{name: "email", maskType: "email", input: "alice@example.com", want: "a***e@example.com"},
		{name: "short email", maskType: "email", input: "a@example.com", want: "***@example.com"},
		{name: "phone", maskType: "phone", input: "13812345678", want: "138****5678"},
		{name: "ipv4", maskType: "ip", input: "192.168.10.25", want: "192.168.*.*"},
		{name: "token", maskType: "token", input: "abcd1234wxyz9876", want: "abcd***9876"},
		{name: "path", maskType: "path", input: "/data/uploads/a.png", want: "***/a.png"},
		{name: "full", maskType: "full", input: "secret", want: "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskValue(tt.maskType, tt.input); got != tt.want {
				t.Fatalf("MaskValue(%q, %q) = %q, want %q", tt.maskType, tt.input, got, tt.want)
			}
		})
	}
}

func TestCloneAndMaskMasksNestedDataWithoutMutatingOriginal(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 30, 0, 0, time.UTC)
	original := samplePayload{
		Email:     "alice@example.com",
		Phone:     "13812345678",
		IP:        "192.168.10.25",
		Path:      "/data/uploads/a.png",
		CreatedAt: now,
		Child: &sampleChild{
			Token: "abcd1234wxyz9876",
		},
		Sessions: []sampleChild{
			{Token: "12345678abcdefgh"},
		},
	}

	got := CloneAndMask(&original, true)
	if got == &original {
		t.Fatal("CloneAndMask should return a cloned pointer")
	}
	if got.Email != "a***e@example.com" {
		t.Fatalf("masked email = %q, want masked value", got.Email)
	}
	if got.Phone != "138****5678" {
		t.Fatalf("masked phone = %q, want masked value", got.Phone)
	}
	if got.IP != "192.168.*.*" {
		t.Fatalf("masked ip = %q, want masked value", got.IP)
	}
	if got.Path != "***/a.png" {
		t.Fatalf("masked path = %q, want masked value", got.Path)
	}
	if got.Child == nil || got.Child.Token != "abcd***9876" {
		t.Fatalf("masked child token = %#v, want masked value", got.Child)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Token != "1234***efgh" {
		t.Fatalf("masked sessions = %#v, want masked slice token", got.Sessions)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, now)
	}

	if original.Email != "alice@example.com" {
		t.Fatalf("original email mutated = %q", original.Email)
	}
	if original.Child == nil || original.Child.Token != "abcd1234wxyz9876" {
		t.Fatalf("original child mutated = %#v", original.Child)
	}
	if len(original.Sessions) != 1 || original.Sessions[0].Token != "12345678abcdefgh" {
		t.Fatalf("original sessions mutated = %#v", original.Sessions)
	}
}

func TestCloneAndMaskAnyLeavesMapPayloadUntouched(t *testing.T) {
	input := map[string]any{
		"email": "alice@example.com",
	}

	got := CloneAndMaskAny(input, true)
	data, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("CloneAndMaskAny() type = %T, want map", got)
	}
	if data["email"] != "alice@example.com" {
		t.Fatalf("map payload email = %#v, want original value", data["email"])
	}
}

type samplePayload struct {
	Email     string `mask:"email"`
	Phone     string `mask:"phone"`
	IP        string `mask:"ip"`
	Path      string `mask:"path"`
	CreatedAt time.Time
	Child     *sampleChild
	Sessions  []sampleChild
}

type sampleChild struct {
	Token string `mask:"token"`
}
