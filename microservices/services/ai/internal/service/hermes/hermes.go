package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	ErrHermesControlPlaneUnavailable = errors.New("Hermes control plane is unavailable")
	ErrHermesInvalidInput            = errors.New("invalid Hermes request")
	ErrHermesPolicyNotConfigured     = errors.New("Hermes policy is not configured")
	// ErrHermesInstanceLimit 是端侧能力限制，不是设计上的禁止：表结构已支持
	// 一节点多实例，等 gak-hermes-manager 支持多 profile 后去掉这道拦截即可。
	ErrHermesInstanceLimit = errors.New("this node already runs an instance; gak-hermes-manager does not support multiple instances per node yet")
)

const (
	defaultHermesDeviceCodeTTL = 10 * time.Minute
	defaultHermesPollInterval  = 5
	// 1000：设备类列表单表无 JOIN，允许整页拉全量后客户端分页；entitlements 等
	// JOIN 页仍只请求 page_size:100，不受此上限影响。
	maxHermesPageSize = 1000
)

var (
	hermesDigestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	hermesErrorCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_.:-]{0,63}$`)
	hermesVersionPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,39}$`)
	// hermesHex32Pattern 匹配 API server 密钥指纹：sha256(key) 前 16 字节的 hex。
	hermesHex32Pattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type HermesPolicySigner struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewHermesPolicySigner accepts a base64-encoded Ed25519 seed or private key.
// An empty value is not an error: callers keep the AI service running and
// expose Hermes as unavailable until a dedicated key is configured.
func NewHermesPolicySigner(encoded string) (*HermesPolicySigner, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	keyBytes, err := decodeHermesBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode Hermes policy signing key: %w", err)
	}
	var privateKey ed25519.PrivateKey
	switch len(keyBytes) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(keyBytes)
	case ed25519.PrivateKeySize:
		privateKey = ed25519.PrivateKey(append([]byte(nil), keyBytes...))
	default:
		return nil, fmt.Errorf("Hermes policy signing key must decode to %d-byte seed or %d-byte private key", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return &HermesPolicySigner{privateKey: privateKey, publicKey: publicKey}, nil
}

// NewHermesPublicKeyVerifier 从 base64 Ed25519 公钥构造只验签的 verifier
// （技能供应链信任根）。空值不是错误：未配置时调用方降级不验签。
func NewHermesPublicKeyVerifier(encoded string) (ed25519.PublicKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	keyBytes, err := decodeHermesBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode Hermes skill signing public key: %w", err)
	}
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("Hermes skill signing public key must decode to %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(append([]byte(nil), keyBytes...)), nil
}

func decodeHermesBase64(value string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
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

func (s *HermesPolicySigner) PublicKeyBase64() string {
	if s == nil {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(s.publicKey)
}

func (s *HermesPolicySigner) SignJSON(value any) (string, error) {
	if s == nil || len(s.privateKey) != ed25519.PrivateKeySize {
		return "", ErrHermesControlPlaneUnavailable
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal Hermes signed payload: %w", err)
	}
	signature := ed25519.Sign(s.privateKey, payload)
	return base64.RawStdEncoding.EncodeToString(signature), nil
}

type HermesServiceOptions struct {
	VerificationURI string
	DeviceCodeTTL   time.Duration
	PollInterval    int
	Now             func() time.Time
	Random          io.Reader
	// SkillSigningPublicKey 是技能供应链验签信任根；nil 时 CreateSkill 降级不验签。
	SkillSigningPublicKey ed25519.PublicKey
}

type HermesService struct {
	dao             *dao.HermesDAO
	signer          *HermesPolicySigner
	verificationURI string
	deviceCodeTTL   time.Duration
	pollInterval    int
	now             func() time.Time
	random          io.Reader
	// 可选：设备吊销 / 权益停用时同步吊销模型 token。用接口注入避免与
	// HermesProxyService 循环依赖；未接线时退化为不吊销（token 仍会自然过期）。
	tokenRevoker HermesModelTokenRevoker
	// 可选：校验策略引用的模型确实存在且启用。未接线时跳过该校验。
	modelLookup HermesModelLookup
	// 可选：官方 Skills Hub 客户端注入（测试指向 httptest）。nil 时按 env 构造。
	hubClient *hermesSkillHubClient
	// 可选：技能供应链验签信任根。nil 时 CreateSkill 降级不验签。
	skillVerifier ed25519.PublicKey
}

// HermesModelLookup 由 AIModelDAO 满足，用于校验策略引用的模型。
type HermesModelLookup interface {
	GetByID(ctx context.Context, id uint64) (*localmodel.AIModel, error)
}

// SetModelLookup 在装配根接线。
func (s *HermesService) SetModelLookup(lookup HermesModelLookup) {
	s.modelLookup = lookup
}

// HermesModelTokenRevoker 由 HermesProxyService 实现。
type HermesModelTokenRevoker interface {
	RevokeDeviceTokens(ctx context.Context, deviceID string) error
	RevokeUserTokens(ctx context.Context, userID uint) error
}

// SetModelTokenRevoker 在装配根接线，使「停用」能立即切断模型访问，
// 而不是等 token 自然过期。
func (s *HermesService) SetModelTokenRevoker(revoker HermesModelTokenRevoker) {
	s.tokenRevoker = revoker
}

func NewHermesService(store *dao.HermesDAO, signer *HermesPolicySigner, options HermesServiceOptions) *HermesService {
	if options.DeviceCodeTTL <= 0 {
		options.DeviceCodeTTL = defaultHermesDeviceCodeTTL
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaultHermesPollInterval
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if strings.TrimSpace(options.VerificationURI) == "" {
		options.VerificationURI = "/hermes/mine"
	}
	return &HermesService{
		dao:             store,
		signer:          signer,
		verificationURI: strings.TrimRight(options.VerificationURI, "/"),
		deviceCodeTTL:   options.DeviceCodeTTL,
		pollInterval:    options.PollInterval,
		now:             options.Now,
		random:          options.Random,
		skillVerifier:   options.SkillSigningPublicKey,
	}
}

func normalizeHermesPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > maxHermesPageSize {
		pageSize = maxHermesPageSize
	}
	return page, pageSize
}

func containsHermesValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
