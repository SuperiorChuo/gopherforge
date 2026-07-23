package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/golang-jwt/jwt/v5"
)

// --- redirect_uri scheme whitelist (L1) ---

func TestIsAllowedRedirectScheme(t *testing.T) {
	allowed := []string{"https", "http", "com.example.app", "org.foo.bar.baz"}
	for _, s := range allowed {
		if !isAllowedRedirectScheme(s) {
			t.Errorf("scheme %q should be allowed", s)
		}
	}
	rejected := []string{"javascript", "data", "vbscript", "file", "ftp", "app", "myscheme"}
	for _, s := range rejected {
		if isAllowedRedirectScheme(s) {
			t.Errorf("scheme %q should be rejected", s)
		}
	}
}

func TestValidateRejectsDangerousRedirectURIs(t *testing.T) {
	svc := OAuth2ClientService{}
	base := ClientMutation{
		Name:       "app",
		ClientType: model.OAuth2ClientConfidential,
		Scopes:     []string{"profile"},
		GrantTypes: []string{model.GrantAuthorizationCode},
	}
	bad := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"https://ok.example.com/cb#frag", // fragment not allowed
		"/relative/path",                 // not absolute
	}
	for _, uri := range bad {
		m := base
		m.RedirectURIs = []string{uri}
		if err := svc.validate(m); err == nil {
			t.Errorf("redirect_uri %q should be rejected", uri)
		}
	}
	// A good one passes.
	m := base
	m.RedirectURIs = []string{"https://app.example.com/callback"}
	if err := svc.validate(m); err != nil {
		t.Errorf("valid https redirect_uri rejected: %v", err)
	}
}

func TestValidatePublicClientCannotUseClientCredentials(t *testing.T) {
	svc := OAuth2ClientService{}
	m := ClientMutation{
		Name:         "spa",
		ClientType:   model.OAuth2ClientPublic,
		RedirectURIs: []string{"https://spa.example.com/cb"},
		Scopes:       []string{"openid"},
		GrantTypes:   []string{model.GrantClientCredentials},
	}
	if err := svc.validate(m); err == nil {
		t.Error("public client with client_credentials should be rejected")
	}
}

func TestValidateRejectsUnsupportedScope(t *testing.T) {
	svc := OAuth2ClientService{}
	m := ClientMutation{
		Name:         "app",
		ClientType:   model.OAuth2ClientConfidential,
		RedirectURIs: []string{"https://app.example.com/cb"},
		Scopes:       []string{"profile", "admin:all"},
		GrantTypes:   []string{model.GrantAuthorizationCode},
	}
	if err := svc.validate(m); err == nil {
		t.Error("unsupported scope admin:all should be rejected")
	}
}

// --- scope subset enforcement (§5) ---

func TestScopesSubset(t *testing.T) {
	allowed := []string{"openid", "profile", "email"}
	if !scopesSubset([]string{"openid", "email"}, allowed) {
		t.Error("subset should be accepted")
	}
	if scopesSubset([]string{"openid", "admin"}, allowed) {
		t.Error("scope not in registered set must be rejected")
	}
	if !scopesSubset(nil, allowed) {
		t.Error("empty requested scope is a trivial subset")
	}
}

func TestParseScopesDedup(t *testing.T) {
	got := parseScopes("openid profile openid  email")
	want := map[string]bool{"openid": true, "profile": true, "email": true}
	if len(got) != 3 {
		t.Fatalf("parseScopes dedup failed: %v", got)
	}
	for _, s := range got {
		if !want[s] {
			t.Errorf("unexpected scope %q", s)
		}
	}
}

// --- PKCE S256 (§2) ---

func TestCodeChallengeS256Matches(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	got := codeChallengeS256(verifier)
	// RFC 7636 appendix B expected challenge.
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got != want {
		t.Errorf("S256 challenge = %q, want %q", got, want)
	}
}

// --- id_token signing + verification (OIDC M2) ---

// testOIDC builds an OIDCService with an in-memory RSA key, bypassing DB.
func testOIDC(t *testing.T) *OIDCService {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &OIDCService{issuerURL: "https://issuer.example.com", priv: priv, kid: keyID(&priv.PublicKey)}
}

func TestSignIDTokenClaimsAndSignature(t *testing.T) {
	oidc := testOIDC(t)
	tok, err := oidc.SignIDToken(t.Context(), IDTokenClaims{
		Subject:  "42",
		Audience: "gak_client123",
		Nonce:    "n-0S6_WzA2Mj",
		TTL:      3600_000_000_000, // 1h in ns
		Extra:    map[string]any{"email": "a@b.com"},
	})
	if err != nil {
		t.Fatalf("SignIDToken: %v", err)
	}
	// Header must pin RS256 + kid.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("id_token not a JWT: %q", tok)
	}
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if !strings.Contains(string(hdr), `"alg":"RS256"`) || !strings.Contains(string(hdr), `"kid"`) {
		t.Errorf("header missing RS256/kid: %s", hdr)
	}
	// Verify signature with the public key + assert claims.
	parsed, err := jwt.Parse(tok, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &oidc.priv.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("id_token verify failed: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iss"] != "https://issuer.example.com/api/v1/oauth2" {
		t.Errorf("iss = %v", claims["iss"])
	}
	if claims["sub"] != "42" || claims["aud"] != "gak_client123" {
		t.Errorf("sub/aud wrong: %v / %v", claims["sub"], claims["aud"])
	}
	if claims["nonce"] != "n-0S6_WzA2Mj" {
		t.Errorf("nonce not echoed: %v", claims["nonce"])
	}
	if claims["email"] != "a@b.com" {
		t.Errorf("email claim missing: %v", claims["email"])
	}
}

func TestSignIDTokenOmitsEmptyNonce(t *testing.T) {
	oidc := testOIDC(t)
	tok, err := oidc.SignIDToken(t.Context(), IDTokenClaims{Subject: "1", Audience: "c", TTL: 3600_000_000_000})
	if err != nil {
		t.Fatalf("SignIDToken: %v", err)
	}
	parts := strings.Split(tok, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if strings.Contains(string(payload), "nonce") {
		t.Errorf("empty nonce should be omitted: %s", payload)
	}
}

// --- Discovery + JWKS (OIDC M2) ---

func TestDiscoveryConsistency(t *testing.T) {
	oidc := testOIDC(t)
	d := oidc.Discovery()
	if d["issuer"] != "https://issuer.example.com/api/v1/oauth2" {
		t.Errorf("issuer = %v", d["issuer"])
	}
	algs, ok := d["id_token_signing_alg_values_supported"].([]string)
	if !ok || len(algs) != 1 || algs[0] != "RS256" {
		t.Errorf("must advertise only RS256, got %v", d["id_token_signing_alg_values_supported"])
	}
	methods, _ := d["code_challenge_methods_supported"].([]string)
	if len(methods) != 1 || methods[0] != "S256" {
		t.Errorf("must advertise only S256, got %v", methods)
	}
	// Every advertised endpoint must sit under the issuer base.
	for _, k := range []string{"authorization_endpoint", "token_endpoint", "userinfo_endpoint", "jwks_uri"} {
		if v, _ := d[k].(string); !strings.HasPrefix(v, "https://issuer.example.com/api/v1/oauth2") {
			t.Errorf("%s not under issuer base: %v", k, d[k])
		}
	}
}

func TestJWKSExposesOnlyPublicKey(t *testing.T) {
	oidc := testOIDC(t)
	jwks, err := oidc.JWKS(t.Context())
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	keys := jwks["keys"].([]map[string]any)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k["kty"] != "RSA" || k["alg"] != "RS256" || k["use"] != "sig" {
		t.Errorf("jwk header fields wrong: %v", k)
	}
	if k["kid"] != oidc.kid {
		t.Errorf("kid mismatch: %v vs %v", k["kid"], oidc.kid)
	}
	// Must NOT leak any private-key component.
	for _, priv := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		if _, present := k[priv]; present {
			t.Errorf("JWKS leaks private component %q", priv)
		}
	}
	// n/e must reconstruct the real public key.
	nBytes, _ := base64.RawURLEncoding.DecodeString(k["n"].(string))
	eBytes, _ := base64.RawURLEncoding.DecodeString(k["e"].(string))
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if n.Cmp(oidc.priv.PublicKey.N) != 0 || int(e.Int64()) != oidc.priv.PublicKey.E {
		t.Error("jwk n/e do not match the signing public key")
	}
}

func TestKeyIDStableForSameKey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	if keyID(&priv.PublicKey) != keyID(&priv.PublicKey) {
		t.Error("keyID must be deterministic for the same key")
	}
	// PEM round-trip must preserve the kid (proves cross-replica stability).
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	block, _ := pem.Decode(pemBytes)
	reloaded, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	if keyID(&priv.PublicKey) != keyID(&reloaded.PublicKey) {
		t.Error("keyID must survive PEM persistence round-trip")
	}
}
