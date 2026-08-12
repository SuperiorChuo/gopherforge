package edgecert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

const (
	// LetsEncrypt production / staging directory.
	dirProduction = "https://acme-v02.api.letsencrypt.org/directory"
	dirStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Issue 对域名执行 ACME HTTP-01 签发，写回 cert 字段（调用方负责落库）。
func Issue(ctx context.Context, cert *Certificate) error {
	if cert == nil {
		return fmt.Errorf("cert is nil")
	}
	domain := strings.TrimSpace(strings.ToLower(cert.Domain))
	email := strings.TrimSpace(cert.Email)
	if domain == "" || email == "" {
		return fmt.Errorf("domain and email are required")
	}
	if strings.ContainsAny(domain, " \t\r\n/") {
		return fmt.Errorf("invalid domain")
	}

	accountKey, err := loadOrCreateAccountKey(cert)
	if err != nil {
		return err
	}

	dirURL := dirProduction
	if cert.IsStaging {
		dirURL = dirStaging
	}
	client := &acme.Client{Key: accountKey, DirectoryURL: dirURL}

	a := &acme.Account{Contact: []string{"mailto:" + email}}
	if _, err := client.Register(ctx, a, acme.AcceptTOS); err != nil {
		// already registered is fine
		if !isAlreadyRegistered(err) {
			return fmt.Errorf("acme register: %w", err)
		}
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return fmt.Errorf("authorize order: %w", err)
	}

	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("get authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for i := range authz.Challenges {
			if authz.Challenges[i].Type == "http-01" {
				chal = authz.Challenges[i]
				break
			}
		}
		if chal == nil {
			return fmt.Errorf("no http-01 challenge for %s", domain)
		}
		keyAuth, err := client.HTTP01ChallengeResponse(chal.Token)
		if err != nil {
			return fmt.Errorf("challenge response: %w", err)
		}
		putChallenge(chal.Token, keyAuth)
		defer deleteChallenge(chal.Token)

		if _, err := client.Accept(ctx, chal); err != nil {
			return fmt.Errorf("accept challenge: %w", err)
		}
		if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
			return fmt.Errorf("wait authorization: %w", err)
		}
	}

	// CSR
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate cert key: %w", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domain},
		DNSNames: []string{domain},
	}, certKey)
	if err != nil {
		return fmt.Errorf("create csr: %w", err)
	}

	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return fmt.Errorf("wait order: %w", err)
	}
	derBundle, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return fmt.Errorf("create order cert: %w", err)
	}
	if len(derBundle) == 0 {
		return fmt.Errorf("empty certificate bundle")
	}

	var fullchain strings.Builder
	for _, der := range derBundle {
		_ = pem.Encode(&fullchain, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	keyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	leaf, err := x509.ParseCertificate(derBundle[0])
	if err != nil {
		return fmt.Errorf("parse leaf: %w", err)
	}
	nb, na := leaf.NotBefore.UTC(), leaf.NotAfter.UTC()
	cert.FullchainPEM = fullchain.String()
	cert.PrivateKeyPEM = string(keyPEM)
	cert.NotBefore = &nb
	cert.NotAfter = &na
	cert.Status = StatusIssued
	cert.LastError = ""
	cert.UpdatedAt = time.Now().UTC()

	if err := writeCertFiles(domain, cert.FullchainPEM, cert.PrivateKeyPEM); err != nil {
		// 落盘失败不判签发失败，仍保存 DB；提示 last_error
		cert.LastError = "issued but write files: " + err.Error()
	}
	return nil
}

func loadOrCreateAccountKey(cert *Certificate) (crypto.Signer, error) {
	if pemStr := strings.TrimSpace(cert.AccountKeyPEM); pemStr != "" {
		block, _ := pem.Decode([]byte(pemStr))
		if block != nil {
			if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return key, nil
			}
			if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				if s, ok := key.(crypto.Signer); ok {
					return s, nil
				}
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	cert.AccountKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	return key, nil
}

func isAlreadyRegistered(err error) bool {
	if err == nil {
		return false
	}
	// acme.Error StatusConflict etc.
	msg := err.Error()
	return strings.Contains(msg, "already registered") || strings.Contains(msg, "409") || strings.Contains(msg, "agree to terms")
}

// writeCertFiles 可选：EDGE_CERT_DIR 下写 domain.crt / domain.key 供 Traefik file 提供方。
func writeCertFiles(domain, fullchain, key string) error {
	dir := strings.TrimSpace(os.Getenv("EDGE_CERT_DIR"))
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	safe := strings.ReplaceAll(domain, "*", "_")
	crt := filepath.Join(dir, safe+".crt")
	keyPath := filepath.Join(dir, safe+".key")
	if err := os.WriteFile(crt, []byte(fullchain), 0o600); err != nil {
		return err
	}
	return os.WriteFile(keyPath, []byte(key), 0o600)
}
