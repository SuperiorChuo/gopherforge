// Package tlsutil 服务间 mTLS 工具：加载证书、构建 TLS credentials。
//
// 内网服务间通信默认明文（insecure），生产部署时启用 mTLS：//
//   1. 证书由 cert-manager / consul-template 签发，挂载到 Pod
//   2. 服务启动时读取证书路径（环境变量 TLS_CERT_PATH / TLS_KEY_PATH / TLS_CA_PATH）
//   3. grpcx.Dial / grpcx 服务注册时自动使用 TLS credentials
//
// 用法：
//
//   // 客户端（拨号时）
//   creds, err := tlsutil.LoadClientCredentials(caPath)
//   conn, err := grpcx.Dial(ctx, resolver, "crm-service", grpc.WithTransportCredentials(creds))
//
//   // 服务端（启动时）
//   srv := grpcx.NewServer(grpc.Creds(creds))
//
// 零信任原则：证书路径为空时回退 insecure（开发模式），生产环境强制校验。
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

type CertPaths struct {
	CertPath string
	KeyPath  string
	CAPath   string
}

func LoadFromEnv() CertPaths {
	return CertPaths{
		CertPath: os.Getenv("TLS_CERT_PATH"),
		KeyPath:  os.Getenv("TLS_KEY_PATH"),
		CAPath:   os.Getenv("TLS_CA_PATH"),
	}
}

func (p CertPaths) IsComplete() bool {
	return p.CertPath != "" && p.KeyPath != "" && p.CAPath != ""
}

func (p CertPaths) HasAny() bool {
	return p.CertPath != "" || p.KeyPath != "" || p.CAPath != ""
}

func LoadServerCredentials(paths CertPaths) (credentials.TransportCredentials, error) {
	if !paths.IsComplete() {
		return nil, fmt.Errorf("tlsutil: incomplete cert paths (cert=%q key=%q ca=%q)",
			paths.CertPath, paths.KeyPath, paths.CAPath)
	}
	cert, err := tls.LoadX509KeyPair(paths.CertPath, paths.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: load server cert: %w", err)
	}
	caPool, err := loadCABundle(paths.CAPath)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	return credentials.NewTLS(config), nil
}

func LoadClientCredentials(caPath string) (credentials.TransportCredentials, error) {
	if caPath == "" {
		return nil, fmt.Errorf("tlsutil: CA path empty")
	}
	caPool, err := loadCABundle(caPath)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
		ServerName: os.Getenv("TLS_SERVER_NAME"),
	}
	return credentials.NewTLS(config), nil
}

func LoadClientCredentialsFromEnv() (credentials.TransportCredentials, error) {
	caPath := os.Getenv("TLS_CA_PATH")
	if caPath == "" {
		return nil, fmt.Errorf("tlsutil: TLS_CA_PATH not set")
	}
	return LoadClientCredentials(caPath)
}

func loadCABundle(caPath string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: read CA bundle %s: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("tlsutil: failed to parse CA bundle %s", caPath)
	}
	return pool, nil
}
