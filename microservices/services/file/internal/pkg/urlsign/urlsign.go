// Package urlsign issues and verifies expiring HMAC signatures for /uploads
// object keys. The /uploads route serves raw objects to browsers (img tags
// cannot attach Authorization headers), so instead of a login check the API
// returns capability URLs: the stored path plus an expiry and an HMAC over
// both. Requests without a valid, unexpired signature are rejected. Signing
// is stateless, so legacy object keys work unchanged.
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	// QueryExpires is the query parameter carrying the unix expiry timestamp.
	QueryExpires = "e"
	// QuerySignature is the query parameter carrying the hex HMAC signature.
	QuerySignature = "s"

	defaultTTL = 15 * time.Minute
)

var (
	ErrNotConfigured = errors.New("url signing secret not configured")
	ErrInvalid       = errors.New("invalid url signature")
	ErrExpired       = errors.New("url signature expired")
)

// Signer signs object keys with an expiry using HMAC-SHA256.
type Signer struct {
	secret []byte
	ttl    time.Duration
}

// New creates a Signer. A non-positive ttl falls back to 15 minutes.
func New(secret string, ttl time.Duration) *Signer {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	secret = strings.TrimSpace(secret)
	var key []byte
	if secret != "" {
		key = []byte(secret)
	}
	return &Signer{secret: key, ttl: ttl}
}

// Enabled reports whether a signing secret is configured.
func (s *Signer) Enabled() bool {
	return s != nil && len(s.secret) > 0
}

// Sign returns the expiry timestamp and signature for an object key.
func (s *Signer) Sign(objectKey string, now time.Time) (expires string, signature string, err error) {
	if !s.Enabled() {
		return "", "", ErrNotConfigured
	}
	expires = strconv.FormatInt(now.Add(s.ttl).Unix(), 10)
	return expires, s.compute(canonicalKey(objectKey), expires), nil
}

// Verify checks the signature and expiry for an object key.
func (s *Signer) Verify(objectKey, expires, signature string, now time.Time) error {
	if !s.Enabled() {
		return ErrNotConfigured
	}
	if expires == "" || signature == "" {
		return ErrInvalid
	}
	expiresAt, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrInvalid
	}
	want := s.compute(canonicalKey(objectKey), expires)
	if !hmac.Equal([]byte(want), []byte(signature)) {
		return ErrInvalid
	}
	if now.Unix() > expiresAt {
		return ErrExpired
	}
	return nil
}

// SignURL appends expiry and signature query parameters to a stored upload
// URL whose path lives under urlPrefix (for example "/uploads"). URLs outside
// the prefix (external CDNs, empty values) are returned unchanged.
func (s *Signer) SignURL(rawURL, urlPrefix string, now time.Time) string {
	if !s.Enabled() || strings.TrimSpace(rawURL) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	prefix := "/" + strings.Trim(urlPrefix, "/")
	if prefix == "/" || !strings.HasPrefix(parsed.Path, prefix+"/") {
		return rawURL
	}
	objectKey := strings.TrimPrefix(parsed.Path, prefix+"/")
	expires, signature, err := s.Sign(objectKey, now)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set(QueryExpires, expires)
	query.Set(QuerySignature, signature)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (s *Signer) compute(objectKey, expires string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(expires))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(objectKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// canonicalKey normalizes an object key so signing and verification agree on
// one spelling regardless of leading slashes or redundant path segments.
func canonicalKey(objectKey string) string {
	objectKey = strings.ReplaceAll(strings.TrimSpace(objectKey), "\\", "/")
	return strings.TrimPrefix(path.Clean("/"+objectKey), "/")
}
