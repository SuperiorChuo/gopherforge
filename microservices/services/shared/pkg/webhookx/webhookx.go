// Package webhookx validates outbound webhook endpoints and signs requests.
// The transport resolves and checks every dial target so a DNS rebinding or
// redirect cannot turn an approved public URL into an SSRF hop.
package webhookx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const SignatureVersion = "v1"

var (
	ErrInvalidEndpoint = errors.New("webhook: invalid endpoint")
	ErrUnsafeEndpoint  = errors.New("webhook: unsafe endpoint")
)

// Policy is secure by default. The two allowances exist for injected test and
// local-development receivers; production composition roots must keep both false.
type Policy struct {
	AllowHTTP    bool
	AllowPrivate bool
	Resolver     *net.Resolver
}

func (p Policy) resolver() *net.Resolver {
	if p.Resolver != nil {
		return p.Resolver
	}
	return net.DefaultResolver
}

// ValidateEndpoint performs syntax and address checks before persistence.
func ValidateEndpoint(ctx context.Context, raw string, policy Policy) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, ErrInvalidEndpoint
	}
	if u.Scheme != "https" && !(policy.AllowHTTP && u.Scheme == "http") {
		return nil, fmt.Errorf("%w: HTTPS is required", ErrInvalidEndpoint)
	}
	if err := validateHost(ctx, u.Hostname(), policy); err != nil {
		return nil, err
	}
	return u, nil
}

func validateHost(ctx context.Context, host string, policy Policy) error {
	_, err := resolvedIPs(ctx, host, policy)
	return err
}

func resolvedIPs(ctx context.Context, host string, policy Policy) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !policy.AllowPrivate && forbiddenIP(ip) {
			return nil, ErrUnsafeEndpoint
		}
		return []net.IP{ip}, nil
	}
	addresses, err := policy.resolver().LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("%w: hostname cannot be resolved", ErrInvalidEndpoint)
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, resolved := range addresses {
		if !policy.AllowPrivate && forbiddenIP(resolved.IP) {
			return nil, ErrUnsafeEndpoint
		}
		ips = append(ips, resolved.IP)
	}
	return ips, nil
}

func forbiddenIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, cidr := range forbiddenCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

var forbiddenCIDRs = parseCIDRs(
	"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24",
	"192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
	"224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "fc00::/7", "fe80::/10",
)

func parseCIDRs(values ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err == nil {
			out = append(out, network)
		}
	}
	return out
}

// Signature computes v1=hex(HMAC-SHA256(secret, timestamp + "." + body)).
func Signature(secret []byte, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return SignatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret []byte, timestamp int64, body []byte, signature string) bool {
	return hmac.Equal([]byte(Signature(secret, timestamp, body)), []byte(signature))
}

// NewClient builds a no-proxy client whose dialer validates the actual resolved
// address. Redirects are rejected to keep signatures and destination binding exact.
func NewClient(policy Policy, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolvedIPs(ctx, host, policy)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
