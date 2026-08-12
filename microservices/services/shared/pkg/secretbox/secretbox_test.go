package secretbox

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestSealOpenAndRandomNonce(t *testing.T) {
	ring := mustKeyring(t, testKey("current", 0x11))
	first, err := ring.Seal([]byte("private material"), "edge-cert:42:private-key")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	second, err := ring.Seal([]byte("private material"), "edge-cert:42:private-key")
	if err != nil {
		t.Fatalf("Seal() second error = %v", err)
	}
	if first == second {
		t.Fatal("Seal() reused a nonce: equal plaintext produced equal envelopes")
	}
	if !strings.HasPrefix(first, "v1.current.") {
		t.Fatalf("Seal() = %q, want versioned current-key envelope", first)
	}

	plaintext, keyID, err := ring.Open(first, "edge-cert:42:private-key")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if string(plaintext) != "private material" || keyID != "current" {
		t.Fatalf("Open() = %q/%q, want private material/current", plaintext, keyID)
	}
}

func TestOpenRejectsTamperingWrongKeyAndAAD(t *testing.T) {
	// Give the previous id the same material: the id itself must still be
	// authenticated, otherwise changing only the envelope header could succeed.
	ring := mustKeyring(t, testKey("current", 0x11), testKey("previous", 0x11))
	envelope, err := ring.Seal([]byte("private material"), "edge-cert:42:private-key")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	parts := strings.Split(envelope, ".")
	tamperedNonce := append([]byte(nil), mustRawURLDecode(t, parts[2])...)
	tamperedNonce[0] ^= 0x01
	tamperedCiphertext := append([]byte(nil), mustRawURLDecode(t, parts[3])...)
	tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0x01
	cases := []struct {
		name     string
		ring     *Keyring
		envelope string
		aad      string
		wantErr  error
	}{
		{name: "nonce", ring: ring, envelope: strings.Join([]string{parts[0], parts[1], base64.RawURLEncoding.EncodeToString(tamperedNonce), parts[3]}, "."), aad: "edge-cert:42:private-key", wantErr: ErrAuthenticationFail},
		{name: "ciphertext", ring: ring, envelope: strings.Join([]string{parts[0], parts[1], parts[2], base64.RawURLEncoding.EncodeToString(tamperedCiphertext)}, "."), aad: "edge-cert:42:private-key", wantErr: ErrAuthenticationFail},
		{name: "key id authenticated", ring: ring, envelope: strings.Join([]string{parts[0], "previous", parts[2], parts[3]}, "."), aad: "edge-cert:42:private-key", wantErr: ErrAuthenticationFail},
		{name: "wrong aad", ring: ring, envelope: envelope, aad: "edge-cert:43:private-key", wantErr: ErrAuthenticationFail},
		{name: "wrong material", ring: mustKeyring(t, testKey("current", 0x99)), envelope: envelope, aad: "edge-cert:42:private-key", wantErr: ErrAuthenticationFail},
		{name: "unknown id", ring: mustKeyring(t, testKey("another", 0x11)), envelope: envelope, aad: "edge-cert:42:private-key", wantErr: ErrUnknownKeyID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plaintext, _, err := tc.ring.Open(tc.envelope, tc.aad)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Open() error = %v, want %v", err, tc.wantErr)
			}
			if plaintext != nil {
				t.Fatalf("Open() returned plaintext after authentication failure: %q", plaintext)
			}
		})
	}
}

func TestOpenRejectsMalformedEnvelope(t *testing.T) {
	ring := mustKeyring(t, testKey("current", 0x11))
	cases := []string{
		"",
		"v2.current.bm9uY2U.Y2lwaGVydGV4dA",
		"v1.current.only-three",
		"v1.bad.id.bm9uY2U.Y2lwaGVydGV4dA",
		"v1.current.***.Y2lwaGVydGV4dA",
		"v1.current.bm9uY2U.***",
		"v1.current.YQ.YQ",
		"v1.current.bm9uY2U=.Y2lwaGVydGV4dA",
	}
	for _, envelope := range cases {
		if _, _, err := ring.Open(envelope, "record:1:field"); !errors.Is(err, ErrMalformedEnvelope) {
			t.Errorf("Open(%q) error = %v, want ErrMalformedEnvelope", envelope, err)
		}
	}
	if _, _, err := ring.Open("anything", ""); !errors.Is(err, ErrEmptyAAD) {
		t.Fatalf("Open(empty AAD) error = %v, want ErrEmptyAAD", err)
	}
	if _, err := ring.Seal([]byte("secret"), ""); !errors.Is(err, ErrEmptyAAD) {
		t.Fatalf("Seal(empty AAD) error = %v, want ErrEmptyAAD", err)
	}
}

func TestNewKeyringValidatesKeys(t *testing.T) {
	cases := []struct {
		name     string
		current  Key
		previous []Key
	}{
		{name: "empty id", current: Key{Material: make([]byte, 32)}},
		{name: "invalid id", current: Key{ID: "bad.id", Material: make([]byte, 32)}},
		{name: "short material", current: Key{ID: "current", Material: make([]byte, 31)}},
		{name: "duplicate id", current: testKey("same", 1), previous: []Key{testKey("same", 2)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewKeyring(tc.current, tc.previous...); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("NewKeyring() error = %v, want ErrInvalidKey", err)
			}
		})
	}
}

func TestRewrapRotatesPreviousEnvelope(t *testing.T) {
	oldRing := mustKeyring(t, testKey("old", 0x22))
	envelope, err := oldRing.Seal([]byte("account key"), "edge-cert:42:account-key")
	if err != nil {
		t.Fatalf("old Seal() error = %v", err)
	}

	rotated := mustKeyring(t, testKey("new", 0x11), testKey("old", 0x22))
	rewrapped, changed, err := rotated.Rewrap(envelope, "edge-cert:42:account-key")
	if err != nil {
		t.Fatalf("Rewrap() error = %v", err)
	}
	if !changed || !strings.HasPrefix(rewrapped, "v1.new.") {
		t.Fatalf("Rewrap() = %q, changed=%v, want current-key envelope", rewrapped, changed)
	}
	plaintext, keyID, err := rotated.Open(rewrapped, "edge-cert:42:account-key")
	if err != nil || string(plaintext) != "account key" || keyID != "new" {
		t.Fatalf("Open(rewrapped) = %q/%q/%v", plaintext, keyID, err)
	}

	unchanged, changed, err := rotated.Rewrap(rewrapped, "edge-cert:42:account-key")
	if err != nil || changed || unchanged != rewrapped {
		t.Fatalf("Rewrap(current) = %q, changed=%v, err=%v", unchanged, changed, err)
	}
}

func mustKeyring(t *testing.T, current Key, previous ...Key) *Keyring {
	t.Helper()
	ring, err := NewKeyring(current, previous...)
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	return ring
}

func testKey(id string, fill byte) Key {
	return Key{ID: id, Material: bytesOf(fill, 32)}
}

func bytesOf(fill byte, count int) []byte {
	value := make([]byte, count)
	for i := range value {
		value[i] = fill
	}
	return value
}

func mustRawURLDecode(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode raw URL base64: %v", err)
	}
	return decoded
}
