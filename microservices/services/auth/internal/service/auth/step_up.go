package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/exportproof"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	// ErrStepUpVerificationFailed intentionally covers every credential/account
	// rejection so callers cannot infer which factor failed.
	ErrStepUpVerificationFailed = errors.New("step-up verification failed")
	// ErrStepUpUnavailable reports a fail-closed infrastructure dependency.
	ErrStepUpUnavailable = errors.New("step-up verification unavailable")
	// ErrStepUpRateLimited reports only that the protected operation was tried
	// too often; it never identifies which factor was rejected.
	ErrStepUpRateLimited = errors.New("too many step-up attempts")
)

const (
	StepUpRateLimitAttempts = 5
	StepUpRateLimitWindow   = 5 * time.Minute
)

var stepUpRateLimitScript = goredis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return current
`)

// EdgeCertificateExportStepUpRequest carries the fresh factors and exact
// resource that will be bound into the one-time export proof.
type EdgeCertificateExportStepUpRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	TOTPCode        string `json:"totp_code,omitempty"`
	CertificateID   uint64 `json:"certificate_id" binding:"required,gt=0"`
}

// EdgeCertificateExportStepUpResponse is safe to return only over the
// authenticated, no-store endpoint. Proof is random, opaque and single-use.
type EdgeCertificateExportStepUpResponse struct {
	Proof            string `json:"proof"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// EdgeCertificateExportStepUpService verifies fresh factors against the
// existing user DAO, then delegates storage to the shared one-time proof store.
type EdgeCertificateExportStepUpService struct {
	users     UserService
	proofs    *exportproof.Store
	redis     *goredis.Client
	available bool
}

// NewEdgeCertificateExportStepUpServiceWithDeps creates a fail-closed service.
// A missing DB or Redis handle never falls back to process-local state.
func NewEdgeCertificateExportStepUpServiceWithDeps(db *gorm.DB, redis *goredis.Client) *EdgeCertificateExportStepUpService {
	service := &EdgeCertificateExportStepUpService{available: db != nil && redis != nil}
	if db != nil {
		service.users = NewUserServiceWithDB(db)
	}
	if redis != nil {
		service.proofs = exportproof.NewStore(redis)
		service.redis = redis
	}
	return service
}

// VerifyAndIssueContext always verifies the password and requires an enrolled
// TOTP factor plus a fresh valid code. Platform-admin status is re-read from the
// database so a stale bearer claim cannot authorize export.
func (s *EdgeCertificateExportStepUpService) VerifyAndIssueContext(
	ctx context.Context,
	userID uint,
	sessionID string,
	req EdgeCertificateExportStepUpRequest,
) (*EdgeCertificateExportStepUpResponse, error) {
	if s == nil || !s.available || s.proofs == nil {
		return nil, ErrStepUpUnavailable
	}
	if userID == 0 || strings.TrimSpace(sessionID) == "" || req.CertificateID == 0 || strings.TrimSpace(req.CurrentPassword) == "" {
		return nil, ErrStepUpVerificationFailed
	}
	if err := s.checkAttemptLimit(ctx, userID, sessionID); err != nil {
		return nil, err
	}

	user, err := s.users.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStepUpVerificationFailed
		}
		return nil, ErrStepUpUnavailable
	}
	if user.Status != 1 || !user.IsPlatformAdmin {
		return nil, ErrStepUpVerificationFailed
	}
	if verifyCurrentPasswordHash(user.Password, req.CurrentPassword) != nil {
		return nil, ErrStepUpVerificationFailed
	}
	// Private-key export is restricted to accounts that have TOTP enabled, and
	// every export requires a fresh code in addition to the password.
	if !user.TOTPEnabled || user.TOTPSecret == "" || !validateTOTPCode(req.TOTPCode, user.TOTPSecret) {
		return nil, ErrStepUpVerificationFailed
	}

	proof, err := s.proofs.Issue(ctx, exportproof.Binding{
		UserID:       uint64(userID),
		SessionID:    sessionID,
		ResourceType: exportproof.ResourceTypeEdgeCertificate,
		ResourceID:   req.CertificateID,
		Audience:     exportproof.AudienceEdgeCertificateExport,
	})
	if err != nil {
		return nil, ErrStepUpUnavailable
	}
	return &EdgeCertificateExportStepUpResponse{
		Proof:            proof,
		ExpiresInSeconds: int64(exportproof.ProofTTL.Seconds()),
	}, nil
}

func (s *EdgeCertificateExportStepUpService) checkAttemptLimit(ctx context.Context, userID uint, sessionID string) error {
	if s == nil || s.redis == nil {
		return ErrStepUpUnavailable
	}
	digest := sha256.Sum256([]byte(sessionID))
	key := fmt.Sprintf("auth:stepup:edge-cert-export:v1:%d:%s", userID, hex.EncodeToString(digest[:]))
	count, err := stepUpRateLimitScript.Run(ctx, s.redis, []string{key}, int64(StepUpRateLimitWindow.Seconds())).Int64()
	if err != nil {
		return ErrStepUpUnavailable
	}
	if count > StepUpRateLimitAttempts {
		return ErrStepUpRateLimited
	}
	return nil
}
