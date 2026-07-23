package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"sync"
	"time"

	systemdao "github.com/go-admin-kit/services/auth/internal/dao/system"
	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// oidcSigningKeySetting is the system_settings key holding the PEM-encoded RSA
// private key used to sign id_tokens. Persisting it lets every auth-service
// replica sign/verify with the same key (and survive restarts) without an
// external secret store.
const oidcSigningKeySetting = "oidc.signing_key"

// OIDCService signs OpenID Connect id_tokens (RS256) and serves the discovery
// document + JWKS. It owns a dedicated RSA keypair, separate from the console
// session HS256 JWT (internal/pkg/jwt), so relying parties can verify id_tokens
// against the public JWKS without any shared secret.
type OIDCService struct {
	settings  *systemdao.SettingDAO
	issuerURL string // public gateway base URL, e.g. http://localhost:8000

	mu   sync.Mutex
	priv *rsa.PrivateKey
	kid  string
}

func NewOIDCService(db *gorm.DB, issuerURL string) *OIDCService {
	return &OIDCService{settings: systemdao.NewSettingDAO(db), issuerURL: issuerURL}
}

// Issuer is the OIDC issuer identifier: a path-scoped URL so discovery/JWKS stay
// under the already-routed /api/v1/oauth2 prefix (no gateway change needed).
func (s *OIDCService) Issuer() string { return s.issuerURL + "/api/v1/oauth2" }

// key lazily loads the RSA key from system_settings, generating and persisting
// one on first use. All replicas converge on a single key via insert-if-absent
// + re-read.
func (s *OIDCService) key(ctx context.Context) (*rsa.PrivateKey, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.priv != nil {
		return s.priv, s.kid, nil
	}

	priv, err := s.loadKey(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		generated, genErr := rsa.GenerateKey(rand.Reader, 2048)
		if genErr != nil {
			return nil, "", genErr
		}
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(generated),
		})
		// Insert only if absent; then re-read so every replica ends up with the
		// same key regardless of who won the generation race.
		if _, createErr := s.settings.CreateIfAbsentContext(ctx, &model.SystemSetting{
			SettingKey: oidcSigningKeySetting,
			ValueJSON:  map[string]any{"pem": string(pemBytes)},
		}); createErr != nil {
			return nil, "", createErr
		}
		priv, err = s.loadKey(ctx)
	}
	if err != nil {
		return nil, "", err
	}

	s.priv = priv
	s.kid = keyID(&priv.PublicKey)
	return s.priv, s.kid, nil
}

func (s *OIDCService) loadKey(ctx context.Context) (*rsa.PrivateKey, error) {
	setting, err := s.settings.GetByKeyContext(ctx, oidcSigningKeySetting)
	if err != nil {
		return nil, err
	}
	raw, _ := setting.ValueJSON["pem"].(string)
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("oidc signing key is not valid PEM")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// keyID derives a stable key id from the public key (SHA-256 of DER), so all
// replicas advertise the same kid for the same key.
func keyID(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "default"
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}

// IDTokenClaims carries the inputs for a signed id_token.
type IDTokenClaims struct {
	Subject  string
	Audience string
	Nonce    string
	TTL      time.Duration
	Extra    map[string]any // scope-gated profile/email claims
}

// SignIDToken mints an RS256 id_token with a kid header.
func (s *OIDCService) SignIDToken(ctx context.Context, c IDTokenClaims) (string, error) {
	priv, kid, err := s.key(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":       s.Issuer(),
		"sub":       c.Subject,
		"aud":       c.Audience,
		"iat":       now.Unix(),
		"exp":       now.Add(c.TTL).Unix(),
		"auth_time": now.Unix(),
	}
	if c.Nonce != "" {
		claims["nonce"] = c.Nonce
	}
	for k, v := range c.Extra {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(priv)
}

// Discovery builds the /.well-known/openid-configuration document.
func (s *OIDCService) Discovery() map[string]any {
	base := s.issuerURL + "/api/v1/oauth2"
	return map[string]any{
		"issuer":                                s.Issuer(),
		"authorization_endpoint":                base + "/authorize",
		"token_endpoint":                        base + "/token",
		"userinfo_endpoint":                     base + "/userinfo",
		"jwks_uri":                              base + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"code_challenge_methods_supported":      []string{"S256"},
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce",
			"name", "preferred_username", "picture", "email",
		},
	}
}

// JWKS builds the public JWK set for id_token verification.
func (s *OIDCService) JWKS(ctx context.Context) (map[string]any, error) {
	priv, kid, err := s.key(ctx)
	if err != nil {
		return nil, err
	}
	pub := priv.PublicKey
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(eBytes),
		}},
	}, nil
}
