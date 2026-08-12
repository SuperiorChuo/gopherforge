// Package secretbox encrypts application secrets with a small, versioned
// AES-256-GCM envelope. Key material is supplied by the composition root; this
// package deliberately does not read environment variables or secret files.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	envelopeVersion = "v1"
	keySize         = 32
)

var (
	ErrInvalidKey         = errors.New("secretbox: invalid key")
	ErrEmptyAAD           = errors.New("secretbox: aad is required")
	ErrMalformedEnvelope  = errors.New("secretbox: malformed envelope")
	ErrUnknownKeyID       = errors.New("secretbox: unknown key id")
	ErrAuthenticationFail = errors.New("secretbox: envelope authentication failed")
)

// Key identifies one AES-256 key in a rotation ring. Material must contain
// exactly 32 bytes (configuration code is responsible for base64 decoding).
type Key struct {
	ID       string
	Material []byte
}

// Keyring seals new values with current and opens values produced by current
// or any explicitly supplied previous key.
type Keyring struct {
	currentID string
	keys      map[string][]byte
	random    io.Reader
}

// NewKeyring validates and defensively copies the rotation ring.
func NewKeyring(current Key, previous ...Key) (*Keyring, error) {
	all := make([]Key, 0, 1+len(previous))
	all = append(all, current)
	all = append(all, previous...)

	keys := make(map[string][]byte, len(all))
	for _, key := range all {
		if !validKeyID(key.ID) || len(key.Material) != keySize {
			return nil, ErrInvalidKey
		}
		if _, exists := keys[key.ID]; exists {
			return nil, ErrInvalidKey
		}
		keys[key.ID] = append([]byte(nil), key.Material...)
	}

	return &Keyring{
		currentID: current.ID,
		keys:      keys,
		random:    rand.Reader,
	}, nil
}

// Seal encrypts plaintext using the current key. aad is mandatory and should
// identify the record and field (for example "edge-cert:42:private-key").
func (r *Keyring) Seal(plaintext []byte, aad string) (string, error) {
	if strings.TrimSpace(aad) == "" {
		return "", ErrEmptyAAD
	}
	if r == nil {
		return "", ErrInvalidKey
	}
	material, ok := r.keys[r.currentID]
	if !ok {
		return "", ErrInvalidKey
	}

	gcm, err := newGCM(material)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(r.random, nonce); err != nil {
		return "", fmt.Errorf("secretbox: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, authenticatedData(r.currentID, aad))

	return strings.Join([]string{
		envelopeVersion,
		r.currentID,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

// Open authenticates and decrypts an envelope. The returned key id lets callers
// observe rotation debt without parsing the envelope themselves.
func (r *Keyring) Open(envelope, aad string) ([]byte, string, error) {
	if strings.TrimSpace(aad) == "" {
		return nil, "", ErrEmptyAAD
	}
	if r == nil {
		return nil, "", ErrInvalidKey
	}

	keyID, nonce, ciphertext, err := parseEnvelope(envelope)
	if err != nil {
		return nil, "", err
	}
	material, ok := r.keys[keyID]
	if !ok {
		return nil, keyID, ErrUnknownKeyID
	}
	gcm, err := newGCM(material)
	if err != nil {
		return nil, keyID, err
	}
	if len(nonce) != gcm.NonceSize() || len(ciphertext) < gcm.Overhead() {
		return nil, keyID, ErrMalformedEnvelope
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, authenticatedData(keyID, aad))
	if err != nil {
		return nil, keyID, ErrAuthenticationFail
	}
	return plaintext, keyID, nil
}

// Rewrap decrypts an envelope and reseals it with current when it was created
// by a previous key. Current envelopes are returned byte-for-byte unchanged.
func (r *Keyring) Rewrap(envelope, aad string) (rewrapped string, changed bool, err error) {
	plaintext, keyID, err := r.Open(envelope, aad)
	if err != nil {
		return "", false, err
	}
	defer clear(plaintext)
	if keyID == r.currentID {
		return envelope, false, nil
	}
	rewrapped, err = r.Seal(plaintext, aad)
	if err != nil {
		return "", false, err
	}
	return rewrapped, true, nil
}

func newGCM(material []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(material)
	if err != nil {
		return nil, ErrInvalidKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: initialize gcm: %w", err)
	}
	return gcm, nil
}

func parseEnvelope(envelope string) (keyID string, nonce, ciphertext []byte, err error) {
	parts := strings.Split(envelope, ".")
	if len(parts) != 4 || parts[0] != envelopeVersion || !validKeyID(parts[1]) || parts[2] == "" || parts[3] == "" {
		return "", nil, nil, ErrMalformedEnvelope
	}
	nonce, err = base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil {
		return "", nil, nil, ErrMalformedEnvelope
	}
	ciphertext, err = base64.RawURLEncoding.Strict().DecodeString(parts[3])
	if err != nil {
		return "", nil, nil, ErrMalformedEnvelope
	}
	return parts[1], nonce, ciphertext, nil
}

func authenticatedData(keyID, aad string) []byte {
	return []byte("secretbox:" + envelopeVersion + ":" + keyID + "\x00" + aad)
}

func validKeyID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}
