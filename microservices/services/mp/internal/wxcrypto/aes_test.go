package wxcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// 生成一个合法的 43 位 EncodingAESKey（base64 of 32 bytes, 去掉尾部 =）。
func genEncodingAESKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	full := base64.StdEncoding.EncodeToString(key) // 44 位，尾部 =
	return full[:len(full)-1]                      // 微信约定 43 位
}

func TestAESRoundTrip(t *testing.T) {
	enc := genEncodingAESKey(t)
	appID := "wxappid123"
	c, err := NewAESCipher(enc, appID)
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}
	msg := []byte("<xml><Content><![CDATA[你好，安全模式]]></Content></xml>")
	var nonce [16]byte
	copy(nonce[:], []byte("0123456789abcdef"))

	encrypted, err := c.Encrypt(msg, nonce[:])
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("round trip mismatch: got %q want %q", got, msg)
	}
}

func TestAESDecryptRejectsWrongAppID(t *testing.T) {
	enc := genEncodingAESKey(t)
	// 用 appid A 加密
	encCipher, _ := NewAESCipher(enc, "wx_correct")
	var nonce [16]byte
	copy(nonce[:], []byte("abcdef0123456789"))
	encrypted, err := encCipher.Encrypt([]byte("<xml/>"), nonce[:])
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// 用 appid B 解密应报 appid 不符
	decCipher, _ := NewAESCipher(enc, "wx_wrong")
	if _, err := decCipher.Decrypt(encrypted); err != ErrAppIDMismatch {
		t.Fatalf("Decrypt with wrong appid: err = %v, want ErrAppIDMismatch", err)
	}
}

func TestNewAESCipherRejectsBadKey(t *testing.T) {
	// 太短的 key（解码后不是 32 字节）
	if _, err := NewAESCipher("dG9vc2hvcnQ", "wxid"); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}

func TestMsgSignatureVerify(t *testing.T) {
	sig := MsgSignature("tok", "1700000000", "nonce1", "encrypted_data")
	if !VerifyMsg("tok", "1700000000", "nonce1", "encrypted_data", sig) {
		t.Fatal("VerifyMsg should accept its own signature")
	}
	if VerifyMsg("tok", "1700000000", "nonce1", "encrypted_data", "wrong") {
		t.Fatal("VerifyMsg should reject wrong signature")
	}
}
