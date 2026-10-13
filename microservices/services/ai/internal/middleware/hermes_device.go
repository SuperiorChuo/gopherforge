package middleware

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	goredis "github.com/redis/go-redis/v9"
)

const (
	HermesDeviceContextKey = "hermes_device"
	hermesMaxSignedBody    = 1024 * 1024
	hermesClockSkew        = 5 * time.Minute
)

var hermesNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

type HermesDeviceIdentityStore interface {
	GetDeviceIdentity(ctx context.Context, deviceID string) (*localmodel.HermesDevice, error)
}

type HermesNonceStore interface {
	Use(ctx context.Context, deviceID, nonce string, ttl time.Duration) (bool, error)
}

type RedisHermesNonceStore struct {
	client goredis.Cmdable
}

func NewRedisHermesNonceStore(client goredis.Cmdable) *RedisHermesNonceStore {
	return &RedisHermesNonceStore{client: client}
}

func (s *RedisHermesNonceStore) Use(ctx context.Context, deviceID, nonce string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("Hermes nonce store is unavailable")
	}
	digest := sha256.Sum256([]byte(nonce))
	key := "hermes:nonce:" + deviceID + ":" + hex.EncodeToString(digest[:])
	return s.client.SetNX(ctx, key, "1", ttl).Result()
}

// HermesCanonicalRequest is the exact UTF-8 payload signed by a manager:
// method, escaped path, raw query, Unix timestamp, nonce and lowercase body
// SHA-256, each separated by a newline.
func HermesCanonicalRequest(method, escapedPath, rawQuery, timestamp, nonce, bodyHash string) []byte {
	return []byte(strings.Join([]string{
		strings.ToUpper(method),
		escapedPath,
		rawQuery,
		timestamp,
		nonce,
		strings.ToLower(bodyHash),
	}, "\n"))
}

func HermesDeviceAuth(store HermesDeviceIdentityStore, nonces HermesNonceStore, now func() time.Time) gin.HandlerFunc {
	if now == nil {
		now = time.Now
	}
	return func(c *gin.Context) {
		deviceID := strings.TrimSpace(c.GetHeader("X-Hermes-Device-ID"))
		timestampValue := strings.TrimSpace(c.GetHeader("X-Hermes-Timestamp"))
		nonce := strings.TrimSpace(c.GetHeader("X-Hermes-Nonce"))
		bodyHashHeader := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Hermes-Body-SHA256")))
		signatureHeader := strings.TrimSpace(c.GetHeader("X-Hermes-Signature"))
		if deviceID == "" || timestampValue == "" || !hermesNoncePattern.MatchString(nonce) ||
			!isHermesSHA256(bodyHashHeader) || signatureHeader == "" {
			hermesAuthFailure(c, "HERMES_DEVICE_AUTH_MISSING", "Hermes device signature headers are invalid")
			return
		}

		timestamp, err := strconv.ParseInt(timestampValue, 10, 64)
		if err != nil {
			hermesAuthFailure(c, "HERMES_DEVICE_AUTH_INVALID", "Hermes request timestamp is invalid")
			return
		}
		requestTime := time.Unix(timestamp, 0)
		currentTime := now()
		if requestTime.Before(currentTime.Add(-hermesClockSkew)) || requestTime.After(currentTime.Add(hermesClockSkew)) {
			hermesAuthFailure(c, "HERMES_DEVICE_AUTH_EXPIRED", "Hermes request timestamp is outside the accepted window")
			return
		}

		body, err := readHermesSignedBody(c.Request)
		if err != nil {
			if err == errHermesBodyTooLarge {
				c.JSON(http.StatusRequestEntityTooLarge, response.Response{Code: http.StatusRequestEntityTooLarge, Message: "Hermes request body is too large", ErrorCode: response.ErrorCode("HERMES_BODY_TOO_LARGE")})
				c.Abort()
				return
			}
			response.InternalServerErrorWithCode(c, response.ErrorCode("HERMES_BODY_READ_FAILED"), err.Error())
			c.Abort()
			return
		}
		computedHash := sha256.Sum256(body)
		computedHashHex := hex.EncodeToString(computedHash[:])
		if subtle.ConstantTimeCompare([]byte(computedHashHex), []byte(bodyHashHeader)) != 1 {
			hermesAuthFailure(c, "HERMES_DEVICE_BODY_MISMATCH", "Hermes request body digest does not match")
			return
		}

		if store == nil {
			response.InternalServerErrorWithCode(c, response.ErrorCode("HERMES_AUTH_UNAVAILABLE"), "Hermes device store is unavailable")
			c.Abort()
			return
		}
		device, err := store.GetDeviceIdentity(c.Request.Context(), deviceID)
		if err != nil || device == nil {
			hermesAuthFailure(c, "HERMES_DEVICE_UNKNOWN", "Hermes device is unknown")
			return
		}
		publicKeyBytes, err := decodeHermesHeaderBase64(device.PublicKey)
		if err != nil || len(publicKeyBytes) != ed25519.PublicKeySize {
			response.InternalServerErrorWithCode(c, response.ErrorCode("HERMES_DEVICE_KEY_INVALID"), "stored Hermes device key is invalid")
			c.Abort()
			return
		}
		signature, err := decodeHermesHeaderBase64(signatureHeader)
		if err != nil || len(signature) != ed25519.SignatureSize {
			hermesAuthFailure(c, "HERMES_DEVICE_SIGNATURE_INVALID", "Hermes device signature is invalid")
			return
		}
		canonical := HermesCanonicalRequest(
			c.Request.Method,
			c.Request.URL.EscapedPath(),
			c.Request.URL.RawQuery,
			timestampValue,
			nonce,
			computedHashHex,
		)
		if !ed25519.Verify(ed25519.PublicKey(publicKeyBytes), canonical, signature) {
			hermesAuthFailure(c, "HERMES_DEVICE_SIGNATURE_INVALID", "Hermes device signature is invalid")
			return
		}

		used, err := nonces.Use(c.Request.Context(), deviceID, nonce, 2*hermesClockSkew)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, response.Response{Code: http.StatusServiceUnavailable, Message: "Hermes replay protection is unavailable", ErrorCode: response.ErrorCode("HERMES_NONCE_STORE_UNAVAILABLE")})
			c.Abort()
			return
		}
		if !used {
			hermesAuthFailure(c, "HERMES_DEVICE_REPLAY", "Hermes request nonce has already been used")
			return
		}
		// 吊销状态仍放在签名与防重放之后判断，避免仅凭猜测 device_id
		// 探测设备是否存在或已吊销。未确认设备只开放三个固定回执端点。
		if device.RevokedAt != nil && (device.RevocationConfirmed || !allowHermesRevocationConfirmation(c.Request)) {
			hermesAuthFailure(c, "HERMES_DEVICE_REVOKED", "Hermes device is revoked")
			return
		}
		c.Set(HermesDeviceContextKey, device)
		c.Set("user_id", device.UserID)
		c.Set("tenant_id", device.TenantID)
		ctx := tenant.WithContext(c.Request.Context(), device.TenantID)
		ctx = context.WithValue(ctx, HermesDeviceContextKey, device)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func allowHermesRevocationConfirmation(request *http.Request) bool {
	if request == nil {
		return false
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/ai/hermes/device/desired-state":
		return true
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/ai/hermes/device/heartbeat":
		return true
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/ai/hermes/device/events":
		return true
	default:
		return false
	}
}

var errHermesBodyTooLarge = fmt.Errorf("Hermes request body exceeds %d bytes", hermesMaxSignedBody)

func readHermesSignedBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return []byte{}, nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, hermesMaxSignedBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > hermesMaxSignedBody {
		return nil, errHermesBodyTooLarge
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func isHermesSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func decodeHermesHeaderBase64(value string) ([]byte, error) {
	encodings := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func hermesAuthFailure(c *gin.Context, code, message string) {
	response.UnauthorizedWithCode(c, response.ErrorCode(code), message)
	c.Abort()
}
