package edgecert

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

const (
	dirProduction = "https://acme-v02.api.letsencrypt.org/directory"
	dirStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

type IssueRequest struct {
	CertificateID   uint64
	Domain          string
	Email           string
	IsStaging       bool
	AccountKeyPEM   []byte `json:"-"`
	CertKeyPEM      []byte `json:"-"`
	OrderURI        string `json:"-"`
	Challenges      ChallengeStore
	PersistProgress func(context.Context, IssueProgress) error `json:"-"`
}

// IssueProgress is durable provider state. Secret values are handed only to
// the worker callback and must never be serialized or logged.
type IssueProgress struct {
	AccountKeyPEM []byte `json:"-"`
	CertKeyPEM    []byte `json:"-"`
	OrderURI      string `json:"-"`
	Step          string
}

type IssueResult struct {
	FullchainPEM      string
	PrivateKeyPEM     []byte `json:"-"`
	AccountKeyPEM     []byte `json:"-"`
	NotBefore         time.Time
	NotAfter          time.Time
	FingerprintSHA256 string
}

// Issuer 可在测试中替换，Worker 不依赖具体 CA。
type Issuer interface {
	Issue(ctx context.Context, req IssueRequest) (IssueResult, error)
}

type ACMEIssuer struct {
	ProductionDirectory string
	StagingDirectory    string
	ChallengeTTL        time.Duration
	clientFactory       acmeClientFactory
}

type acmeClient interface {
	Register(context.Context, *acme.Account, func(string) bool) (*acme.Account, error)
	AuthorizeOrder(context.Context, []acme.AuthzID, ...acme.OrderOption) (*acme.Order, error)
	GetOrder(context.Context, string) (*acme.Order, error)
	GetAuthorization(context.Context, string) (*acme.Authorization, error)
	HTTP01ChallengeResponse(string) (string, error)
	Accept(context.Context, *acme.Challenge) (*acme.Challenge, error)
	WaitAuthorization(context.Context, string) (*acme.Authorization, error)
	WaitOrder(context.Context, string) (*acme.Order, error)
	CreateOrderCert(context.Context, string, []byte, bool) ([][]byte, string, error)
	FetchCert(context.Context, string, bool) ([][]byte, error)
}

type acmeClientFactory func(crypto.Signer, string) acmeClient

type providerOperationError struct {
	operation string
	cause     error
}

func (e *providerOperationError) Error() string {
	return "certificate authority " + e.operation + " failed"
}

func (e *providerOperationError) Unwrap() error { return e.cause }

func providerFailure(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &providerOperationError{operation: operation, cause: err}
}

func (i ACMEIssuer) newClient(key crypto.Signer, directory string) acmeClient {
	if i.clientFactory != nil {
		return i.clientFactory(key, directory)
	}
	return &acme.Client{Key: key, DirectoryURL: directory}
}

func (i ACMEIssuer) Issue(ctx context.Context, req IssueRequest) (result IssueResult, resultErr error) {
	domain, err := canonicalDomain(req.Domain)
	if err != nil {
		return IssueResult{}, err
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		return IssueResult{}, fmt.Errorf("valid email is required")
	}
	if req.CertificateID == 0 || req.Challenges == nil {
		return IssueResult{}, fmt.Errorf("durable challenge store is required")
	}
	if req.PersistProgress == nil {
		return IssueResult{}, fmt.Errorf("durable issuance progress callback is required")
	}
	if len(bytes.TrimSpace(req.AccountKeyPEM)) == 0 || len(bytes.TrimSpace(req.CertKeyPEM)) == 0 {
		return IssueResult{}, fmt.Errorf("issuer requires worker-persisted key material before provider access")
	}

	accountKey, accountPEM, err := loadOrCreateAccountKey(req.AccountKeyPEM)
	if err != nil {
		return IssueResult{}, err
	}
	defer clear(accountPEM)
	certKey, certPEM, err := loadOrCreateCertificateKey(req.CertKeyPEM)
	if err != nil {
		return IssueResult{}, err
	}
	defer clear(certPEM)
	if err := req.PersistProgress(ctx, IssueProgress{
		AccountKeyPEM: accountPEM,
		CertKeyPEM:    certPEM,
	}); err != nil {
		return IssueResult{}, err
	}
	directory := i.ProductionDirectory
	if directory == "" {
		directory = dirProduction
	}
	if req.IsStaging {
		directory = i.StagingDirectory
		if directory == "" {
			directory = dirStaging
		}
	}
	client := i.newClient(accountKey, directory)
	if _, err := client.Register(ctx, &acme.Account{Contact: []string{"mailto:" + email}}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return IssueResult{}, providerFailure("account registration", err)
	}

	orderURI := strings.TrimSpace(req.OrderURI)
	var order *acme.Order
	if orderURI != "" {
		if !validProviderReference(directory, orderURI) {
			return IssueResult{}, fmt.Errorf("invalid stored provider order reference")
		}
		order, err = client.GetOrder(ctx, orderURI)
		if err != nil {
			return IssueResult{}, providerFailure("order lookup", err)
		}
		if order == nil {
			return IssueResult{}, providerFailure("order lookup", errors.New("empty order response"))
		}
		if order.URI == "" {
			order.URI = orderURI
		}
	} else {
		if err := req.PersistProgress(ctx, IssueProgress{
			AccountKeyPEM: accountPEM,
			CertKeyPEM:    certPEM,
			Step:          "order_creating",
		}); err != nil {
			return IssueResult{}, err
		}
		order, err = client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
		if err != nil {
			return IssueResult{}, ErrProviderStateUncertain
		}
		if order == nil || strings.TrimSpace(order.URI) == "" || !validProviderReference(directory, order.URI) {
			return IssueResult{}, ErrProviderStateUncertain
		}
		orderURI = strings.TrimSpace(order.URI)
		if err := req.PersistProgress(ctx, IssueProgress{
			AccountKeyPEM: accountPEM,
			CertKeyPEM:    certPEM,
			OrderURI:      orderURI,
			Step:          "order_created",
		}); err != nil {
			return IssueResult{}, ErrProviderStateUncertain
		}
	}
	if providerOrderTerminal(order, time.Now().UTC()) {
		return IssueResult{}, ErrProviderOrderTerminal
	}

	if order.Status == acme.StatusPending {
		if err := i.completeAuthorizations(ctx, client, req, order); err != nil {
			return IssueResult{}, err
		}
		order, err = client.WaitOrder(ctx, orderURI)
		if err != nil {
			return IssueResult{}, providerOrderFailure("order readiness wait", err)
		}
	} else if order.Status == acme.StatusProcessing {
		order, err = client.WaitOrder(ctx, orderURI)
		if err != nil {
			return IssueResult{}, providerOrderFailure("order processing wait", err)
		}
	}
	if providerOrderTerminal(order, time.Now().UTC()) {
		return IssueResult{}, ErrProviderOrderTerminal
	}

	// Go 1.26 ECDSA signing with a nil randomness source uses RFC 6979. This
	// makes the DER CSR byte-for-byte stable when a finalization is retried.
	csrDER, err := x509.CreateCertificateRequest(nil, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
	}, certKey)
	if err != nil {
		return IssueResult{}, fmt.Errorf("create csr: %w", err)
	}

	var derBundle [][]byte
	switch order.Status {
	case acme.StatusValid:
		if strings.TrimSpace(order.CertURL) == "" {
			return IssueResult{}, providerFailure("certificate retrieval", errors.New("valid order has no certificate reference"))
		}
		derBundle, err = client.FetchCert(ctx, order.CertURL, true)
		if err != nil {
			return IssueResult{}, providerFailure("certificate retrieval", err)
		}
	case acme.StatusReady:
		if strings.TrimSpace(order.FinalizeURL) == "" {
			return IssueResult{}, providerFailure("order finalization", errors.New("ready order has no finalize reference"))
		}
		if err := req.PersistProgress(ctx, IssueProgress{
			AccountKeyPEM: accountPEM,
			CertKeyPEM:    certPEM,
			OrderURI:      orderURI,
			Step:          "finalizing",
		}); err != nil {
			return IssueResult{}, err
		}
		derBundle, _, err = client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
		if err != nil {
			return IssueResult{}, providerOrderFailure("order finalization", err)
		}
	default:
		return IssueResult{}, providerFailure("order state validation", errors.New("order is not ready for certificate retrieval"))
	}
	if len(derBundle) == 0 {
		return IssueResult{}, providerFailure("certificate retrieval", errors.New("empty certificate bundle"))
	}
	var fullchain strings.Builder
	for _, der := range derBundle {
		if err := pem.Encode(&fullchain, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			return IssueResult{}, fmt.Errorf("encode certificate: %w", err)
		}
	}
	leaf, err := x509.ParseCertificate(derBundle[0])
	if err != nil {
		return IssueResult{}, fmt.Errorf("parse leaf certificate: %w", err)
	}
	return IssueResult{
		FullchainPEM:  fullchain.String(),
		PrivateKeyPEM: append([]byte(nil), certPEM...),
		AccountKeyPEM: append([]byte(nil), accountPEM...),
		NotBefore:     leaf.NotBefore.UTC(), NotAfter: leaf.NotAfter.UTC(),
		FingerprintSHA256: fingerprintDER(leaf.Raw),
	}, nil
}

func providerOrderTerminal(order *acme.Order, now time.Time) bool {
	if order == nil {
		return false
	}
	if order.Status == acme.StatusInvalid {
		return true
	}
	return order.Status != acme.StatusValid && !order.Expires.IsZero() && !order.Expires.After(now)
}

func providerOrderFailure(operation string, err error) error {
	var orderErr *acme.OrderError
	if errors.As(err, &orderErr) && orderErr.Status == acme.StatusInvalid {
		return ErrProviderOrderTerminal
	}
	return providerFailure(operation, err)
}

func (i ACMEIssuer) completeAuthorizations(ctx context.Context, client acmeClient, req IssueRequest, order *acme.Order) error {
	ttl := i.ChallengeTTL
	if ttl <= 0 {
		ttl = challengeTTL
	}
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return providerFailure("authorization lookup", err)
		}
		if authz == nil {
			return providerFailure("authorization lookup", errors.New("empty authorization response"))
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		challenge := http01Challenge(authz)
		if challenge == nil {
			return fmt.Errorf("no http-01 challenge is available")
		}
		keyAuth, err := client.HTTP01ChallengeResponse(challenge.Token)
		if err != nil {
			return providerFailure("challenge preparation", err)
		}
		if err := req.Challenges.Put(ctx, req.CertificateID, challenge.Token, keyAuth, time.Now().UTC().Add(ttl)); err != nil {
			return fmt.Errorf("persist challenge: %w", err)
		}
		err = func() error {
			defer func() { _ = req.Challenges.Delete(context.WithoutCancel(ctx), challenge.Token) }()
			if _, err := client.Accept(ctx, challenge); err != nil {
				return providerFailure("challenge acceptance", err)
			}
			if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
				return providerFailure("authorization wait", err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func validProviderReference(directory, reference string) bool {
	directoryURL, err := url.Parse(directory)
	if err != nil || directoryURL.Scheme == "" || directoryURL.Host == "" {
		return false
	}
	referenceURL, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || referenceURL.Scheme == "" || referenceURL.Host == "" || referenceURL.User != nil || referenceURL.Fragment != "" {
		return false
	}
	return strings.EqualFold(directoryURL.Scheme, referenceURL.Scheme) && strings.EqualFold(directoryURL.Host, referenceURL.Host)
}

func canonicalDomain(domain string) (string, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(domain)), ".")
	if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, " \t\r\n/\\:*") {
		return "", fmt.Errorf("invalid domain")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("domain must be fully qualified")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid domain")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", fmt.Errorf("invalid domain")
			}
		}
	}
	return domain, nil
}

func http01Challenge(authz *acme.Authorization) *acme.Challenge {
	for _, challenge := range authz.Challenges {
		if challenge.Type == "http-01" {
			return challenge
		}
	}
	return nil
}

func loadOrCreateAccountKey(existing []byte) (crypto.Signer, []byte, error) {
	if len(bytes.TrimSpace(existing)) != 0 {
		block, rest := pem.Decode(existing)
		if block == nil || len(bytes.TrimSpace(rest)) != 0 {
			return nil, nil, fmt.Errorf("invalid account key PEM")
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return key, append([]byte(nil), existing...), nil
		}
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			if signer, ok := key.(crypto.Signer); ok {
				return signer, append([]byte(nil), existing...), nil
			}
		}
		return nil, nil, fmt.Errorf("unsupported account key")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate account key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal account key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return key, pemBytes, nil
}

func loadOrCreateCertificateKey(existing []byte) (crypto.Signer, []byte, error) {
	if len(bytes.TrimSpace(existing)) != 0 {
		block, rest := pem.Decode(existing)
		if block == nil || len(bytes.TrimSpace(rest)) != 0 {
			return nil, nil, fmt.Errorf("invalid certificate key PEM")
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return key, append([]byte(nil), existing...), nil
		}
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			if signer, ok := key.(*ecdsa.PrivateKey); ok {
				return signer, append([]byte(nil), existing...), nil
			}
		}
		return nil, nil, fmt.Errorf("unsupported certificate key")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal certificate key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return key, pemBytes, nil
}

func fingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func parseLeaf(fullchain string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(fullchain))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	return leaf, nil
}
