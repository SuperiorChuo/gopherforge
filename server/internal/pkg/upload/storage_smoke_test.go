package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/config"
)

func TestObjectStorageSmokeOpenReadsRealEndpoint(t *testing.T) {
	if os.Getenv("BLACK8_OBJECT_STORAGE_SMOKE") != "1" {
		t.Skip("set BLACK8_OBJECT_STORAGE_SMOKE=1 to run the real object storage smoke test")
	}

	smokeCfg := readObjectStorageSmokeConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	objectKey := smokeCfg.objectKey()
	content := fmt.Sprintf("black8 object storage smoke\nprovider=%s\nkey=%s\n", smokeCfg.provider, objectKey)
	t.Logf("object storage smoke provider=%s endpoint=%s bucket=%s region_present=%t use_ssl=%t object_key=%s",
		smokeCfg.provider,
		smokeCfg.storage.Endpoint,
		smokeCfg.storage.Bucket,
		strings.TrimSpace(smokeCfg.storage.Region) != "",
		smokeCfg.storage.UseSSL,
		objectKey,
	)

	provider, err := NewStorageProvider(smokeCfg.uploadConfig())
	if err != nil {
		t.Fatalf("new storage provider: %v", err)
	}
	stored, err := provider.Store(ctx, objectKey, strings.NewReader(content))
	if err != nil {
		t.Fatalf("store smoke object through provider: %v", err)
	}
	if stored.FilePath != objectKey {
		t.Fatalf("stored file path = %q, want %q", stored.FilePath, objectKey)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := provider.Delete(cleanupCtx, objectKey); err != nil {
			t.Logf("cleanup smoke object %q failed: %v", objectKey, err)
		}
	})

	opened, err := provider.Open(ctx, objectKey)
	if err != nil {
		t.Fatalf("open smoke object through provider: %v", err)
	}
	defer opened.Body.Close()

	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read smoke object body: %v", err)
	}
	if string(body) != content {
		t.Fatalf("read content = %q, want %q", string(body), content)
	}
	if opened.Key != objectKey {
		t.Fatalf("opened key = %q, want %q", opened.Key, objectKey)
	}
	if opened.StorageType != smokeCfg.provider {
		t.Fatalf("opened storage type = %q, want %q", opened.StorageType, smokeCfg.provider)
	}
	if opened.Size != int64(len(content)) {
		t.Fatalf("opened size = %d, want %d", opened.Size, len(content))
	}

	missingKey := objectKey + ".missing"
	_, err = provider.Open(ctx, missingKey)
	if !isRecognizableMissingObjectOpenError(err, missingKey) {
		t.Fatalf("missing object open error = %v, want recognizable missing-object failure", err)
	}
}

type objectStorageSmokeConfig struct {
	provider string
	storage  config.ObjectStorageConfig
	prefix   string
}

func readObjectStorageSmokeConfig(t *testing.T) objectStorageSmokeConfig {
	t.Helper()

	provider := strings.ToLower(strings.TrimSpace(firstEnv("BLACK8_OBJECT_STORAGE_PROVIDER", "BLACK8_OBJECT_STORAGE_TYPE")))
	if provider == "" {
		provider = "minio"
	}
	if provider != "s3" && provider != "minio" {
		t.Fatalf("BLACK8_OBJECT_STORAGE_PROVIDER must be s3 or minio, got %q", provider)
	}

	endpoint := requiredSmokeEnv(t, "BLACK8_OBJECT_STORAGE_ENDPOINT")
	bucket := requiredSmokeEnv(t, "BLACK8_OBJECT_STORAGE_BUCKET")
	region := strings.TrimSpace(os.Getenv("BLACK8_OBJECT_STORAGE_REGION"))
	accessKey := requiredSmokeEnv(t, "BLACK8_OBJECT_STORAGE_ACCESS_KEY")
	secretKey := requiredSmokeEnv(t, "BLACK8_OBJECT_STORAGE_SECRET_KEY")
	if provider == "s3" && region == "" {
		t.Fatal("BLACK8_OBJECT_STORAGE_REGION is required when BLACK8_OBJECT_STORAGE_PROVIDER=s3")
	}

	defaultUseSSL := provider == "s3"
	useSSL, err := smokeBoolEnv("BLACK8_OBJECT_STORAGE_USE_SSL", defaultUseSSL)
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.TrimSpace(os.Getenv("BLACK8_OBJECT_STORAGE_OBJECT_PREFIX"))
	if prefix == "" {
		prefix = "black8-smoke"
	}
	prefix, err = cleanObjectKey(prefix)
	if err != nil {
		t.Fatalf("BLACK8_OBJECT_STORAGE_OBJECT_PREFIX is invalid: %v", err)
	}

	return objectStorageSmokeConfig{
		provider: provider,
		storage: config.ObjectStorageConfig{
			Endpoint:  endpoint,
			Bucket:    bucket,
			Region:    region,
			AccessKey: accessKey,
			SecretKey: secretKey,
			UseSSL:    useSSL,
		},
		prefix: prefix,
	}
}

func (c objectStorageSmokeConfig) uploadConfig() config.UploadConfig {
	uploadCfg := config.UploadConfig{
		StorageType:   c.provider,
		PublicBaseURL: "https://object-storage-smoke.invalid",
	}
	switch c.provider {
	case "s3":
		uploadCfg.S3 = c.storage
	case "minio":
		uploadCfg.MinIO = c.storage
	}
	return uploadCfg
}

func (c objectStorageSmokeConfig) objectKey() string {
	return fmt.Sprintf("%s/control-plane-open-smoke/%d.txt", c.prefix, time.Now().UTC().UnixNano())
}

func requiredSmokeEnv(t *testing.T, key string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s is required for object storage smoke test", key)
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func smokeBoolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean value, got %q", key, value)
	}
	return parsed, nil
}

func isRecognizableMissingObjectOpenError(err error, objectKey string) bool {
	if err == nil || !errors.Is(err, ErrStorageProviderUnavailable) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, strings.ToLower(objectKey)) {
		return false
	}
	markers := []string{
		"404",
		"does not exist",
		"no such key",
		"nosuchkey",
		"not found",
		"specified key",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
