package edgecert

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
)

const (
	recoveryTestDirectory = "https://acme.test/directory"
	recoveryTestOrderURI  = "https://acme.test/orders/42"
	recoveryTestFinalize  = "https://acme.test/orders/42/finalize"
	recoveryTestCertURL   = "https://acme.test/certificates/42"
)

type recoveryTestChallengeStore struct{}

func (recoveryTestChallengeStore) Put(context.Context, uint64, string, string, time.Time) error {
	return nil
}

func (recoveryTestChallengeStore) Delete(context.Context, string) error { return nil }

type recoveryTestACMEClient struct {
	order          *acme.Order
	registerErr    error
	authorizeOrder *acme.Order
	authorizeErr   error

	registerCalls       int
	authorizeOrderCalls int
	getOrderCalls       int
	waitOrderCalls      int
	createOrderCalls    int
	fetchCertCalls      int
	getOrderURIs        []string
	getOrderErrors      []error
	createFinalizeURLs  []string
	createCSRs          [][]byte
	fetchCertURLs       []string
	certificateBundle   [][]byte
}

func (c *recoveryTestACMEClient) Register(context.Context, *acme.Account, func(string) bool) (*acme.Account, error) {
	c.registerCalls++
	return &acme.Account{}, c.registerErr
}

func (c *recoveryTestACMEClient) AuthorizeOrder(context.Context, []acme.AuthzID, ...acme.OrderOption) (*acme.Order, error) {
	c.authorizeOrderCalls++
	if c.authorizeErr != nil {
		return nil, c.authorizeErr
	}
	if c.authorizeOrder != nil {
		order := *c.authorizeOrder
		return &order, nil
	}
	return nil, errors.New("unexpected AuthorizeOrder call")
}

func (c *recoveryTestACMEClient) GetOrder(_ context.Context, orderURI string) (*acme.Order, error) {
	c.getOrderCalls++
	c.getOrderURIs = append(c.getOrderURIs, orderURI)
	if c.getOrderCalls <= len(c.getOrderErrors) && c.getOrderErrors[c.getOrderCalls-1] != nil {
		return nil, c.getOrderErrors[c.getOrderCalls-1]
	}
	if c.order == nil {
		return nil, errors.New("test order is not configured")
	}
	order := *c.order
	return &order, nil
}

func (*recoveryTestACMEClient) GetAuthorization(context.Context, string) (*acme.Authorization, error) {
	return nil, errors.New("unexpected GetAuthorization call")
}

func (*recoveryTestACMEClient) HTTP01ChallengeResponse(string) (string, error) {
	return "", errors.New("unexpected HTTP01ChallengeResponse call")
}

func (*recoveryTestACMEClient) Accept(context.Context, *acme.Challenge) (*acme.Challenge, error) {
	return nil, errors.New("unexpected Accept call")
}

func (*recoveryTestACMEClient) WaitAuthorization(context.Context, string) (*acme.Authorization, error) {
	return nil, errors.New("unexpected WaitAuthorization call")
}

func (c *recoveryTestACMEClient) WaitOrder(context.Context, string) (*acme.Order, error) {
	c.waitOrderCalls++
	return nil, errors.New("unexpected WaitOrder call")
}

func (c *recoveryTestACMEClient) CreateOrderCert(_ context.Context, finalizeURL string, csr []byte, _ bool) ([][]byte, string, error) {
	c.createOrderCalls++
	c.createFinalizeURLs = append(c.createFinalizeURLs, finalizeURL)
	c.createCSRs = append(c.createCSRs, append([]byte(nil), csr...))
	return c.certificateBundle, recoveryTestCertURL, nil
}

func (c *recoveryTestACMEClient) FetchCert(_ context.Context, certURL string, _ bool) ([][]byte, error) {
	c.fetchCertCalls++
	c.fetchCertURLs = append(c.fetchCertURLs, certURL)
	return c.certificateBundle, nil
}

func TestACMEIssuerExistingOrderUsesGetOrderWithoutAuthorize(t *testing.T) {
	req, certificateDER, _ := recoveryTestIssueRequest(t)
	client := &recoveryTestACMEClient{
		order: &acme.Order{
			URI:         recoveryTestOrderURI,
			Status:      acme.StatusReady,
			FinalizeURL: recoveryTestFinalize,
		},
		certificateBundle: [][]byte{certificateDER},
	}
	issuer := recoveryTestIssuer(client)

	if _, err := issuer.Issue(context.Background(), req); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if client.getOrderCalls != 1 || len(client.getOrderURIs) != 1 || client.getOrderURIs[0] != recoveryTestOrderURI {
		t.Fatalf("GetOrder calls = %d, URIs = %q; want one lookup of stored URI", client.getOrderCalls, client.getOrderURIs)
	}
	if client.authorizeOrderCalls != 0 {
		t.Fatalf("AuthorizeOrder calls = %d; want 0 for recovered order", client.authorizeOrderCalls)
	}
	if client.createOrderCalls != 1 {
		t.Fatalf("CreateOrderCert calls = %d; want 1 for ready order", client.createOrderCalls)
	}
}

func TestACMEIssuerAcceptsAccountAlreadyExistsSentinel(t *testing.T) {
	req, certificateDER, _ := recoveryTestIssueRequest(t)
	client := &recoveryTestACMEClient{
		registerErr: acme.ErrAccountAlreadyExists,
		order: &acme.Order{
			URI:         recoveryTestOrderURI,
			Status:      acme.StatusReady,
			FinalizeURL: recoveryTestFinalize,
		},
		certificateBundle: [][]byte{certificateDER},
	}
	issuer := recoveryTestIssuer(client)

	if _, err := issuer.Issue(context.Background(), req); err != nil {
		t.Fatalf("Issue() rejected acme.ErrAccountAlreadyExists: %v", err)
	}
	if client.registerCalls != 1 || client.getOrderCalls != 1 || client.authorizeOrderCalls != 0 {
		t.Fatalf("recovery calls after account sentinel = register:%d get:%d authorize:%d", client.registerCalls, client.getOrderCalls, client.authorizeOrderCalls)
	}
}

func TestACMEIssuerValidRecoveredOrderFetchesCertificate(t *testing.T) {
	req, certificateDER, progress := recoveryTestIssueRequest(t)
	client := &recoveryTestACMEClient{
		order: &acme.Order{
			URI:     recoveryTestOrderURI,
			Status:  acme.StatusValid,
			CertURL: recoveryTestCertURL,
		},
		certificateBundle: [][]byte{certificateDER},
	}
	issuer := recoveryTestIssuer(client)

	result, err := issuer.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if client.fetchCertCalls != 1 || len(client.fetchCertURLs) != 1 || client.fetchCertURLs[0] != recoveryTestCertURL {
		t.Fatalf("FetchCert calls = %d, URLs = %q; want one fetch of certificate URL", client.fetchCertCalls, client.fetchCertURLs)
	}
	if client.createOrderCalls != 0 {
		t.Fatalf("CreateOrderCert calls = %d; want 0 for valid order", client.createOrderCalls)
	}
	if client.authorizeOrderCalls != 0 {
		t.Fatalf("AuthorizeOrder calls = %d; want 0 for recovered order", client.authorizeOrderCalls)
	}
	if result.FingerprintSHA256 != fingerprintDER(certificateDER) {
		t.Fatalf("fingerprint = %q; want %q", result.FingerprintSHA256, fingerprintDER(certificateDER))
	}
	for _, update := range *progress {
		if update.Step == "finalizing" {
			t.Fatal("valid order must not persist a finalizing step")
		}
	}
}

func TestACMEIssuerReadyOrderRetryReusesExactCSR(t *testing.T) {
	req, certificateDER, progress := recoveryTestIssueRequest(t)
	client := &recoveryTestACMEClient{
		order: &acme.Order{
			URI:         recoveryTestOrderURI,
			Status:      acme.StatusReady,
			FinalizeURL: recoveryTestFinalize,
		},
		certificateBundle: [][]byte{certificateDER},
	}
	issuer := recoveryTestIssuer(client)

	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := issuer.Issue(context.Background(), req); err != nil {
			t.Fatalf("Issue() attempt %d error = %v", attempt, err)
		}
	}
	if client.authorizeOrderCalls != 0 {
		t.Fatalf("AuthorizeOrder calls = %d; want 0 for retries of recovered order", client.authorizeOrderCalls)
	}
	if client.createOrderCalls != 2 || len(client.createCSRs) != 2 {
		t.Fatalf("CreateOrderCert calls = %d, CSRs = %d; want 2", client.createOrderCalls, len(client.createCSRs))
	}
	if !bytes.Equal(client.createCSRs[0], client.createCSRs[1]) {
		t.Fatal("CreateOrderCert received different CSR bytes for the same persisted certificate key and order")
	}
	csr, err := x509.ParseCertificateRequest(client.createCSRs[0])
	if err != nil {
		t.Fatalf("ParseCertificateRequest() error = %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature error = %v", err)
	}
	if len(csr.DNSNames) != 1 || csr.DNSNames[0] != req.Domain {
		t.Fatalf("CSR DNSNames = %q; want [%q]", csr.DNSNames, req.Domain)
	}
	finalizingUpdates := 0
	for _, update := range *progress {
		if update.Step == "finalizing" {
			finalizingUpdates++
			if update.OrderURI != recoveryTestOrderURI {
				t.Fatalf("finalizing progress OrderURI = %q; want %q", update.OrderURI, recoveryTestOrderURI)
			}
		}
	}
	if finalizingUpdates != 2 {
		t.Fatalf("finalizing progress updates = %d; want 2", finalizingUpdates)
	}
}

func recoveryTestIssuer(client acmeClient) ACMEIssuer {
	return ACMEIssuer{
		ProductionDirectory: recoveryTestDirectory,
		clientFactory: func(_ crypto.Signer, _ string) acmeClient {
			return client
		},
	}
}

func recoveryTestIssueRequest(t *testing.T) (IssueRequest, []byte, *[]IssueProgress) {
	t.Helper()
	_, accountKeyPEM, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatalf("loadOrCreateAccountKey() error = %v", err)
	}
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatalf("loadOrCreateCertificateKey() error = %v", err)
	}
	certificateDER := recoveryTestCertificateDER(t, certKeyPEM, "admin.chouai.cc.cd")
	progress := new([]IssueProgress)
	return IssueRequest{
		CertificateID: 1,
		Domain:        "admin.chouai.cc.cd",
		Email:         "ops@example.com",
		AccountKeyPEM: accountKeyPEM,
		CertKeyPEM:    certKeyPEM,
		OrderURI:      recoveryTestOrderURI,
		Challenges:    recoveryTestChallengeStore{},
		PersistProgress: func(_ context.Context, update IssueProgress) error {
			update.AccountKeyPEM = append([]byte(nil), update.AccountKeyPEM...)
			update.CertKeyPEM = append([]byte(nil), update.CertKeyPEM...)
			*progress = append(*progress, update)
			return nil
		},
	}, certificateDER, progress
}

func recoveryTestCertificateDER(t *testing.T, certKeyPEM []byte, domain string) []byte {
	t.Helper()
	signer, _, err := loadOrCreateCertificateKey(certKeyPEM)
	if err != nil {
		t.Fatalf("load persisted certificate key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return certificateDER
}
