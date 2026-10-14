package authjwt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

var (
	ErrUnauthorized = errors.New("invalid agent credentials")
	ErrForbidden    = errors.New("IM permission required")
	ErrUnavailable  = errors.New("agent verification unavailable")
)

// Verifier delegates access-token validity, revocation and current permissions
// to auth-service. A valid JWT signature alone never grants the agent role.
// It uses the existing ForwardAuth endpoint, not a second permission store.
type Verifier struct {
	endpoint string
	client   *http.Client
}

func NewVerifier(base string) *Verifier {
	v := &Verifier{}
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return v // Invalid/missing configuration fails closed at the use site.
	}
	u.Path = "/internal/verify"
	v.endpoint = u.String()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Never send user credentials through an ambient proxy.
	transport.MaxResponseHeaderBytes = 64 << 10
	v.client = &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // Do not forward credentials on redirects.
		},
	}
	return v
}

func (v *Verifier) VerifyAgent(ctx context.Context, token string) (*AgentClaims, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthorized
	}
	if v == nil || v.endpoint == "" || v.client == nil {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusForbidden:
		return nil, ErrForbidden
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%w (status %d)", ErrUnavailable, resp.StatusCode)
	}
	uid, uidErr := strconv.ParseUint(resp.Header.Get(gatewayauth.HeaderUserID), 10, 64)
	tid, tidErr := strconv.ParseUint(resp.Header.Get("X-Auth-Tenant-ID"), 10, 64)
	if uidErr != nil || tidErr != nil || uid == 0 || tid == 0 {
		// Anonymous or malformed owner response: an access token must always
		// resolve to a concrete tenant-scoped user before IM trusts it.
		return nil, ErrUnauthorized
	}
	permissions := gatewayauth.ParsePermissions(resp.Header.Get(gatewayauth.HeaderPermissions))
	for _, code := range []string{"im:agent:access", "im:admin:manage", "*"} {
		if _, ok := permissions[code]; ok {
			return &AgentClaims{UserID: uid, TenantID: tid, Username: resp.Header.Get("X-Auth-Username")}, nil
		}
	}
	return nil, ErrForbidden
}
