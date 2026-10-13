package middleware

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

type hermesDeviceStoreStub struct {
	device *localmodel.HermesDevice
	err    error
}

func (s hermesDeviceStoreStub) GetDeviceIdentity(context.Context, string) (*localmodel.HermesDevice, error) {
	return s.device, s.err
}

type hermesNonceStoreStub struct {
	mu   sync.Mutex
	used map[string]struct{}
}

func (s *hermesNonceStoreStub) Use(_ context.Context, deviceID, nonce string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deviceID + ":" + nonce
	if _, exists := s.used[key]; exists {
		return false, nil
	}
	s.used[key] = struct{}{}
	return true, nil
}

func signedHermesRequest(t *testing.T, privateKey ed25519.PrivateKey, method, target string, body []byte, deviceID string, now time.Time, nonce string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	timestamp := strconv.FormatInt(now.Unix(), 10)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	canonical := HermesCanonicalRequest(method, request.URL.EscapedPath(), request.URL.RawQuery, timestamp, nonce, digestHex)
	signature := ed25519.Sign(privateKey, canonical)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Hermes-Device-ID", deviceID)
	request.Header.Set("X-Hermes-Timestamp", timestamp)
	request.Header.Set("X-Hermes-Nonce", nonce)
	request.Header.Set("X-Hermes-Body-SHA256", digestHex)
	request.Header.Set("X-Hermes-Signature", base64.RawStdEncoding.EncodeToString(signature))
	return request
}

func setupHermesDeviceAuthRouter(t *testing.T, now time.Time, revokedAt ...*time.Time) (*gin.Engine, ed25519.PrivateKey, *hermesNonceStoreStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	nonces := &hermesNonceStoreStub{used: make(map[string]struct{})}
	device := &localmodel.HermesDevice{
		DeviceID:  "11111111-2222-4333-8444-555555555555",
		TenantID:  7,
		UserID:    9,
		PublicKey: base64.RawStdEncoding.EncodeToString(publicKey),
	}
	if len(revokedAt) > 0 {
		device.RevokedAt = revokedAt[0]
	}
	store := hermesDeviceStoreStub{device: device}
	router := gin.New()
	auth := HermesDeviceAuth(store, nonces, func() time.Time { return now })
	handler := func(c *gin.Context) {
		deviceValue, exists := c.Get(HermesDeviceContextKey)
		device, ok := deviceValue.(*localmodel.HermesDevice)
		if !exists || !ok || device.DeviceID != store.device.DeviceID {
			c.Status(http.StatusInternalServerError)
			return
		}
		if tenant.FromContext(c.Request.Context()) != 7 {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	}
	router.GET("/api/v1/ai/hermes/device/desired-state", auth, handler)
	router.POST("/api/v1/ai/hermes/device/heartbeat", auth, handler)
	router.POST("/api/v1/ai/hermes/device/events", auth, handler)
	router.POST("/api/v1/ai/hermes/device/model-token", auth, handler)
	return router, privateKey, nonces
}

func TestHermesDeviceAuthAllowsRevokedDeviceFinalControlMessages(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	router, privateKey, _ := setupHermesDeviceAuthRouter(t, now, &revokedAt)
	request := signedHermesRequest(
		t,
		privateKey,
		http.MethodPost,
		"/api/v1/ai/hermes/device/heartbeat",
		[]byte(`{"actual_state":"revoked"}`),
		"11111111-2222-4333-8444-555555555555",
		now,
		"revokeddevice001",
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("revoked final heartbeat status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	blocked := signedHermesRequest(
		t,
		privateKey,
		http.MethodPost,
		"/api/v1/ai/hermes/device/model-token",
		[]byte(`{}`),
		"11111111-2222-4333-8444-555555555555",
		now,
		"revokeddevice002",
	)
	blockedRecorder := httptest.NewRecorder()
	router.ServeHTTP(blockedRecorder, blocked)
	if blockedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked model-token status = %d, want 401", blockedRecorder.Code)
	}
}

func TestHermesDeviceAuthRejectsAllRequestsAfterRevocationConfirmation(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	router, privateKey, _ := setupHermesDeviceAuthRouter(t, now, &revokedAt)

	// Rebuild the helper's device state through a dedicated router because the
	// returned store is intentionally hidden from callers.
	publicKey := privateKey.Public().(ed25519.PublicKey)
	store := hermesDeviceStoreStub{device: &localmodel.HermesDevice{
		DeviceID:            "11111111-2222-4333-8444-555555555555",
		TenantID:            7,
		UserID:              9,
		PublicKey:           base64.RawStdEncoding.EncodeToString(publicKey),
		RevokedAt:           &revokedAt,
		ActualState:         localmodel.HermesDesiredStateRevoked,
		RevocationConfirmed: true,
	}}
	nonces := &hermesNonceStoreStub{used: make(map[string]struct{})}
	router = gin.New()
	router.GET("/api/v1/ai/hermes/device/desired-state", HermesDeviceAuth(store, nonces, func() time.Time { return now }), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := signedHermesRequest(
		t,
		privateKey,
		http.MethodGet,
		"/api/v1/ai/hermes/device/desired-state",
		nil,
		"11111111-2222-4333-8444-555555555555",
		now,
		"revokedconfirmed1",
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("confirmed revoked desired-state status = %d, want 401", recorder.Code)
	}
}

func TestHermesDeviceAuthAllowsPermanentRevocationAfterPolicyRevokedState(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	store := hermesDeviceStoreStub{device: &localmodel.HermesDevice{
		DeviceID:    "11111111-2222-4333-8444-555555555555",
		TenantID:    7,
		UserID:      9,
		PublicKey:   base64.RawStdEncoding.EncodeToString(publicKey),
		RevokedAt:   &revokedAt,
		ActualState: localmodel.HermesDesiredStateRevoked,
	}}
	router := gin.New()
	router.GET(
		"/api/v1/ai/hermes/device/desired-state",
		HermesDeviceAuth(store, &hermesNonceStoreStub{used: make(map[string]struct{})}, func() time.Time { return now }),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	request := signedHermesRequest(
		t,
		privateKey,
		http.MethodGet,
		"/api/v1/ai/hermes/device/desired-state",
		nil,
		store.device.DeviceID,
		now,
		"policyrevoked001",
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unconfirmed policy-revoked desired-state status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHermesDeviceAuthVerifiesSignatureAndRejectsReplay(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	router, privateKey, _ := setupHermesDeviceAuthRouter(t, now)
	body := []byte(`{"actual_state":"active"}`)
	const deviceID = "11111111-2222-4333-8444-555555555555"
	const nonce = "abcdefghijklmnop"

	request := signedHermesRequest(t, privateKey, http.MethodPost, "/api/v1/ai/hermes/device/heartbeat?source=manager", body, deviceID, now, nonce)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	replay := signedHermesRequest(t, privateKey, http.MethodPost, "/api/v1/ai/hermes/device/heartbeat?source=manager", body, deviceID, now, nonce)
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401; body=%s", replayRecorder.Code, replayRecorder.Body.String())
	}
}

func TestHermesDeviceAuthRejectsTamperedBodyBeforeUsingNonce(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	router, privateKey, nonces := setupHermesDeviceAuthRouter(t, now)
	request := signedHermesRequest(
		t,
		privateKey,
		http.MethodPost,
		"/api/v1/ai/hermes/device/heartbeat",
		[]byte(`{"actual_state":"active"}`),
		"11111111-2222-4333-8444-555555555555",
		now,
		"tamperedbody0001",
	)
	request.Body = ioNopCloserForHermesTest([]byte(`{"actual_state":"stopped"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("tampered request status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	nonces.mu.Lock()
	usedCount := len(nonces.used)
	nonces.mu.Unlock()
	if usedCount != 0 {
		t.Fatalf("nonce store used %d entries before signature validation, want 0", usedCount)
	}
}

func TestHermesDeviceAuthRejectsExpiredTimestamp(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	router, privateKey, _ := setupHermesDeviceAuthRouter(t, now)
	request := signedHermesRequest(
		t,
		privateKey,
		http.MethodPost,
		"/api/v1/ai/hermes/device/heartbeat",
		[]byte(`{}`),
		"11111111-2222-4333-8444-555555555555",
		now.Add(-hermesClockSkew-time.Second),
		"expiredtimestamp1",
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expired request status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func ioNopCloserForHermesTest(body []byte) *nopReadCloser {
	return &nopReadCloser{Reader: bytes.NewReader(body)}
}

type nopReadCloser struct {
	*bytes.Reader
}

func (nopReadCloser) Close() error { return nil }
