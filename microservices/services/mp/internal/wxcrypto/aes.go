package wxcrypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// 安全模式（AES）消息加解密，实现微信公众平台的 AES-256-CBC 方案。
// 关键约定（与随机 IV 的常规 CBC 不同）：
//   - AESKey = base64decode(EncodingAESKey + "=")，必须 32 字节（AES-256）
//   - IV = AESKey 的前 16 字节（固定，非随机）
//   - 明文结构：[16 字节随机] + [4 字节 msg_len 大端] + [msg] + [appid]
//     解密后尾部 appid 必须与本账号 appid 一致（防串号）

var (
	ErrInvalidAESKey  = errors.New("wxcrypto: EncodingAESKey 解码后不是 32 字节")
	ErrInvalidPadding = errors.New("wxcrypto: PKCS7 填充非法")
	ErrAppIDMismatch  = errors.New("wxcrypto: 密文尾部 appid 与账号不符")
	ErrCipherTooShort = errors.New("wxcrypto: 密文长度不足")
	ErrPlaintextShort = errors.New("wxcrypto: 明文结构长度不足")
)

// AESCipher 持有解码后的 32 字节 key 与 16 字节 IV。
type AESCipher struct {
	key   []byte
	iv    []byte
	appID string
}

// NewAESCipher 从 EncodingAESKey（43 位 base64，微信不带尾部 "="）与账号 appid
// 构造。encodingAESKey 补 "=" 后 base64 解码，须得 32 字节。
func NewAESCipher(encodingAESKey, appID string) (*AESCipher, error) {
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("wxcrypto: base64 解码 EncodingAESKey: %w", err)
	}
	if len(key) != 32 {
		return nil, ErrInvalidAESKey
	}
	return &AESCipher{key: key, iv: key[:16], appID: appID}, nil
}

// Decrypt 解密微信 <Encrypt> 密文，返回内部消息 XML 明文。
func (a *AESCipher) Decrypt(encrypted string) ([]byte, error) {
	cipherData, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("wxcrypto: base64 解码密文: %w", err)
	}
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	if len(cipherData) < aes.BlockSize || len(cipherData)%aes.BlockSize != 0 {
		return nil, ErrCipherTooShort
	}
	plain := make([]byte, len(cipherData))
	cipher.NewCBCDecrypter(block, a.iv).CryptBlocks(plain, cipherData)

	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return nil, err
	}
	// 结构：16 随机 + 4 长度(大端) + msg + appid
	if len(plain) < 20 {
		return nil, ErrPlaintextShort
	}
	msgLen := binary.BigEndian.Uint32(plain[16:20])
	if 20+int(msgLen) > len(plain) {
		return nil, ErrPlaintextShort
	}
	msg := plain[20 : 20+int(msgLen)]
	gotAppID := string(plain[20+int(msgLen):])
	if a.appID != "" && gotAppID != a.appID {
		return nil, ErrAppIDMismatch
	}
	return msg, nil
}

// Encrypt 加密回复消息明文，返回 base64 的 <Encrypt> 内容。nonce16 为 16 字节
// 随机前缀；为可测传入（生产用随机，见 EncryptWithRandom）。
func (a *AESCipher) Encrypt(msg []byte, nonce16 []byte) (string, error) {
	if len(nonce16) != 16 {
		return "", fmt.Errorf("wxcrypto: nonce 必须 16 字节")
	}
	var buf bytes.Buffer
	buf.Write(nonce16)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(msg)))
	buf.Write(lenBuf)
	buf.Write(msg)
	buf.WriteString(a.appID)

	padded := pkcs7Pad(buf.Bytes(), aes.BlockSize)
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, a.iv).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	if pad == 0 {
		pad = blockSize
	}
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	n := len(data)
	if n == 0 || n%aes.BlockSize != 0 {
		return nil, ErrInvalidPadding
	}
	pad := int(data[n-1])
	if pad < 1 || pad > aes.BlockSize || pad > n {
		return nil, ErrInvalidPadding
	}
	for _, b := range data[n-pad:] {
		if int(b) != pad {
			return nil, ErrInvalidPadding
		}
	}
	return data[:n-pad], nil
}
