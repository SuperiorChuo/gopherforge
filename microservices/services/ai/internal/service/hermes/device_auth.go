package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/google/uuid"
	"io"
	"strings"
	"time"
)

type CreateHermesDeviceAuthorizationRequest struct {
	PublicKey  string `json:"public_key"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	Arch       string `json:"arch"`
}

type HermesDeviceAuthorizationResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type HermesDeviceTokenResponse struct {
	DeviceID               string    `json:"device_id"`
	PolicySigningPublicKey string    `json:"policy_signing_public_key"`
	IssuedAt               time.Time `json:"issued_at"`
}

func (s *HermesService) CreateDeviceAuthorization(ctx context.Context, request CreateHermesDeviceAuthorizationRequest) (*HermesDeviceAuthorizationResponse, error) {
	publicKey, encodedPublicKey, err := parseHermesPublicKey(request.PublicKey)
	if err != nil {
		return nil, err
	}
	request.DeviceName = strings.TrimSpace(request.DeviceName)
	if len(request.DeviceName) > 80 {
		return nil, fmt.Errorf("%w: device_name is too long", ErrHermesInvalidInput)
	}
	request.Platform = strings.ToLower(strings.TrimSpace(request.Platform))
	request.Arch = strings.ToLower(strings.TrimSpace(request.Arch))
	if !containsHermesValue([]string{"windows", "darwin", "linux"}, request.Platform) {
		return nil, fmt.Errorf("%w: unsupported platform", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{"amd64", "arm64"}, request.Arch) {
		return nil, fmt.Errorf("%w: unsupported architecture", ErrHermesInvalidInput)
	}

	deviceCode, err := randomHermesToken(s.random, 32)
	if err != nil {
		return nil, err
	}
	userCode, err := randomHermesUserCode(s.random)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	fingerprint := sha256.Sum256(publicKey)
	code := &localmodel.HermesDeviceCode{
		DeviceCodeHash: hashHermesDeviceCode(deviceCode),
		UserCodeHash:   hashHermesUserCode(userCode),
		PublicKey:      encodedPublicKey,
		KeyFingerprint: hex.EncodeToString(fingerprint[:]),
		DeviceName:     request.DeviceName,
		Platform:       request.Platform,
		Arch:           request.Arch,
		Status:         localmodel.HermesDeviceCodePending,
		ExpiresAt:      now.Add(s.deviceCodeTTL),
		CreatedAt:      now,
	}
	if err := s.dao.CreateDeviceCode(ctx, code); err != nil {
		return nil, err
	}
	return &HermesDeviceAuthorizationResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: s.verificationURI,
		ExpiresIn:       int(s.deviceCodeTTL.Seconds()),
		Interval:        s.pollInterval,
	}, nil
}

func (s *HermesService) ApproveDeviceAuthorization(ctx context.Context, tenantID, userID uint, userCode string) error {
	if tenantID == 0 || userID == 0 {
		return fmt.Errorf("%w: authenticated user is required", ErrHermesInvalidInput)
	}
	normalized := normalizeHermesUserCode(userCode)
	if len(normalized) != 8 {
		return fmt.Errorf("%w: invalid user code", ErrHermesInvalidInput)
	}
	return s.dao.ApproveDeviceCode(ctx, hashHermesUserCode(normalized), tenantID, userID, s.now().UTC())
}

func (s *HermesService) ExchangeDeviceCode(ctx context.Context, deviceCode string) (*HermesDeviceTokenResponse, error) {
	if s.signer == nil {
		return nil, ErrHermesControlPlaneUnavailable
	}
	deviceCode = strings.TrimSpace(deviceCode)
	if len(deviceCode) < 32 || len(deviceCode) > 128 {
		return nil, fmt.Errorf("%w: invalid device code", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	device := &localmodel.HermesDevice{
		DeviceID:       uuid.NewString(),
		ActualState:    "registered",
		HealthStatus:   "unknown",
		LastErrorCode:  "",
		ManagerVersion: "",
		HermesVersion:  "",
		ConfigDigest:   "",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.dao.ConsumeDeviceCode(ctx, hashHermesDeviceCode(deviceCode), now, device); err != nil {
		return nil, err
	}
	return &HermesDeviceTokenResponse{
		DeviceID:               device.DeviceID,
		PolicySigningPublicKey: s.signer.PublicKeyBase64(),
		IssuedAt:               now,
	}, nil
}

func parseHermesPublicKey(value string) (ed25519.PublicKey, string, error) {
	decoded, err := decodeHermesBase64(strings.TrimSpace(value))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, "", fmt.Errorf("%w: public_key must be a base64 Ed25519 public key", ErrHermesInvalidInput)
	}
	return ed25519.PublicKey(decoded), base64.RawStdEncoding.EncodeToString(decoded), nil
}

func randomHermesToken(reader io.Reader, size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", fmt.Errorf("generate Hermes random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func randomHermesUserCode(reader io.Reader) (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buffer := make([]byte, 8)
	random := make([]byte, len(buffer))
	if _, err := io.ReadFull(reader, random); err != nil {
		return "", fmt.Errorf("generate Hermes user code: %w", err)
	}
	for index := range buffer {
		buffer[index] = alphabet[int(random[index])%len(alphabet)]
	}
	return string(buffer[:4]) + "-" + string(buffer[4:]), nil
}

func normalizeHermesUserCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.NewReplacer("-", "", " ", "").Replace(value)
}

func hashHermesDeviceCode(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func hashHermesUserCode(value string) string {
	digest := sha256.Sum256([]byte(normalizeHermesUserCode(value)))
	return hex.EncodeToString(digest[:])
}
