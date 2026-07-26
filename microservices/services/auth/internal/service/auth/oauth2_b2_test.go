package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"
)

// --- per-client 配额 ---

// fakeRateRedis 只实现限流用到的两个命令，用来精确控制 INCR 的返回计数。
type fakeRateRedis struct {
	count    int64
	incrErr  error
	expErr   error
	incrKeys []string
}

func (f *fakeRateRedis) Incr(_ context.Context, key string) *goredis.IntCmd {
	f.incrKeys = append(f.incrKeys, key)
	if f.incrErr != nil {
		return goredis.NewIntResult(0, f.incrErr)
	}
	f.count++
	return goredis.NewIntResult(f.count, nil)
}

func (f *fakeRateRedis) ExpireNX(_ context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	if f.expErr != nil {
		return goredis.NewBoolResult(false, f.expErr)
	}
	return goredis.NewBoolResult(true, nil)
}

func TestEffectiveTokenRateFallsBackToDefault(t *testing.T) {
	if got := EffectiveTokenRate(&model.OAuth2Client{TokenRatePerMinute: 0}); got != DefaultTokenRatePerMinute {
		t.Errorf("rate 0 should fall back to default, got %d", got)
	}
	if got := EffectiveTokenRate(&model.OAuth2Client{TokenRatePerMinute: 5}); got != 5 {
		t.Errorf("per-client rate should win, got %d", got)
	}
	if got := EffectiveTokenRate(nil); got != DefaultTokenRatePerMinute {
		t.Errorf("nil client should yield default, got %d", got)
	}
}

func TestCheckTokenRateBlocksOnlyBeyondQuota(t *testing.T) {
	fake := &fakeRateRedis{}
	svc := &OAuth2ServerService{redis: fake}
	client := &model.OAuth2Client{ClientID: "gak_abc", TokenRatePerMinute: 3}

	// 配额内的每一次都必须放行（含正好第 3 次——边界不能提前拦）。
	for i := 1; i <= 3; i++ {
		if err := svc.CheckTokenRate(t.Context(), client); err != nil {
			t.Fatalf("call %d within quota rejected: %v", i, err)
		}
	}
	// 第 4 次越界。
	err := svc.CheckTokenRate(t.Context(), client)
	if err == nil {
		t.Fatal("call beyond quota should be rejected")
	}
	if err.Status != 429 || err.Code != "slow_down" {
		t.Errorf("want 429/slow_down, got %d/%s", err.Status, err.Code)
	}
	if err.RetryAfter <= 0 {
		t.Error("rate limit error must carry Retry-After")
	}
	// key 必须按 client_id 分桶，不同 client 不能互相耗配额。
	for _, k := range fake.incrKeys {
		if !strings.Contains(k, "gak_abc") {
			t.Errorf("rate key not scoped to client: %q", k)
		}
	}
}

// Redis 故障必须放行：限流是防滥用而非鉴权，缓存挂了不该打死正常对接。
func TestCheckTokenRateFailsOpen(t *testing.T) {
	client := &model.OAuth2Client{ClientID: "gak_abc", TokenRatePerMinute: 1}
	cases := map[string]*OAuth2ServerService{
		"redis 未配置":   {redis: nil},
		"INCR 报错":     {redis: &fakeRateRedis{incrErr: errors.New("boom")}},
		"ExpireNX 报错": {redis: &fakeRateRedis{expErr: errors.New("boom")}},
	}
	for name, svc := range cases {
		if err := svc.CheckTokenRate(t.Context(), client); err != nil {
			t.Errorf("%s 应放行，却被拦：%v", name, err)
		}
	}
}

func TestCheckTokenRateIgnoresNilClient(t *testing.T) {
	svc := &OAuth2ServerService{redis: &fakeRateRedis{}}
	if err := svc.CheckTokenRate(t.Context(), nil); err != nil {
		t.Errorf("nil client should be a no-op, got %v", err)
	}
}

// --- JWT 形态 access token（RFC 9068）---

func TestSignAccessTokenRFC9068Shape(t *testing.T) {
	oidc := testOIDC(t)
	tok, err := oidc.SignAccessToken(t.Context(), AccessTokenClaims{
		Subject:  "42",
		Audience: "gak_client123",
		ClientID: "gak_client123",
		Scope:    "openid profile",
		JTI:      "jti-abc",
		TenantID: 7,
		Username: "alice",
		TTL:      time.Hour,
	})
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("access token not a JWT: %q", tok)
	}
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	// typ=at+jwt 是 RFC 9068 §2.1 的要求：资源服务器据此拒收被当作
	// access token 递过来的 id_token。
	if !strings.Contains(string(hdr), `"typ":"at+jwt"`) {
		t.Errorf("header must pin typ=at+jwt: %s", hdr)
	}
	if !strings.Contains(string(hdr), `"alg":"RS256"`) || !strings.Contains(string(hdr), `"kid"`) {
		t.Errorf("header missing RS256/kid: %s", hdr)
	}
	// 必须能用 JWKS 暴露的同一把公钥离线验签——这正是 JWT 形态的意义。
	parsed, err := jwt.Parse(tok, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &oidc.priv.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("access token verify failed: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	for key, want := range map[string]any{
		"iss":       "https://issuer.example.com/api/v1/oauth2",
		"sub":       "42",
		"aud":       "gak_client123",
		"client_id": "gak_client123",
		"scope":     "openid profile",
		"jti":       "jti-abc",
		"username":  "alice",
	} {
		if claims[key] != want {
			t.Errorf("claim %s = %v, want %v", key, claims[key], want)
		}
	}
	if claims["exp"] == nil || claims["iat"] == nil {
		t.Error("exp/iat must be present")
	}
}

// 没有签名密钥时必须拒绝签发，绝不静默退回 opaque：调用方按形态决定校验
// 方式，静默降级会让它把随机串当 JWT 校验并放行。
func TestMintJWTAccessTokenRefusesWithoutSigningKey(t *testing.T) {
	svc := &OAuth2ServerService{oidc: nil}
	client := &model.OAuth2Client{
		ClientID:          "gak_abc",
		AccessTokenTTL:    3600,
		AccessTokenFormat: model.AccessTokenFormatJWT,
	}
	_, err := svc.mintAccessToken(t.Context(), client, nil, "", 1, []string{"openid"}, model.GrantClientCredentials, nil)
	if err == nil {
		t.Fatal("jwt format without OIDC key must fail, not silently downgrade")
	}
	if err.Status != 500 || err.Code != "server_error" {
		t.Errorf("want 500/server_error, got %d/%s", err.Status, err.Code)
	}
}

// --- 客户端配置校验 ---

func TestValidateAccessTokenFormatAndRate(t *testing.T) {
	base := func() ClientMutation {
		return ClientMutation{
			Name:         "demo",
			ClientType:   model.OAuth2ClientConfidential,
			RedirectURIs: []string{"https://app.example.com/cb"},
			Scopes:       []string{"openid"},
			GrantTypes:   []string{model.GrantAuthorizationCode},
		}
	}
	svc := OAuth2ClientService{}

	// 空值（= opaque）、opaque、jwt 都必须接受。
	for _, format := range []string{"", model.AccessTokenFormatOpaque, model.AccessTokenFormatJWT} {
		m := base()
		m.AccessTokenFormat = format
		if err := svc.validate(m); err != nil {
			t.Errorf("format %q should be valid: %v", format, err)
		}
	}
	m := base()
	m.AccessTokenFormat = "bearer-ish"
	if err := svc.validate(m); err == nil {
		t.Error("unknown token format must be rejected")
	}
	m = base()
	m.TokenRatePerMinute = -1
	if err := svc.validate(m); err == nil {
		t.Error("negative rate must be rejected")
	}
}

// 未指定形态时归一化为 opaque——库里的列有 NOT NULL 约束，空串会写坏行。
func TestNormalizeTTLsDefaultsTokenFormat(t *testing.T) {
	m := ClientMutation{}
	normalizeTTLs(&m)
	if m.AccessTokenFormat != model.AccessTokenFormatOpaque {
		t.Errorf("format should default to opaque, got %q", m.AccessTokenFormat)
	}
}
