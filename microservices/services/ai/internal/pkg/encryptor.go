package pkg

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Encryptor AES-256-GCM 加密器（用于安全存储敏感信息如 API Key）
type Encryptor struct {
	key []byte // 32字节密钥
}

// NewEncryptor 创建加密器。key 必须是32字节（AES-256）。
func NewEncryptor(key string) (*Encryptor, error) {
	keyBytes := []byte(key)
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(keyBytes))
	}
	return &Encryptor{key: keyBytes}, nil
}

// Encrypt 加密明文，返回 (密文base64, IV的hex, error)
func (e *Encryptor) Encrypt(plaintext string) (string, string, error) {
	if plaintext == "" {
		return "", "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	// 生成随机 nonce（IV）
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	// 加密
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// 返回 base64 编码的密文和 nonce
	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(nonce),
		nil
}

// Decrypt 解密密文
func (e *Encryptor) Decrypt(ciphertextB64, nonceB64 string) (string, error) {
	if ciphertextB64 == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(nonce) != gcm.NonceSize() {
		return "", errors.New("invalid nonce size")
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
