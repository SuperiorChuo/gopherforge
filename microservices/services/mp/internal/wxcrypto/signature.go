// Package wxcrypto implements 微信公众号 callback signature verification.
package wxcrypto

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
)

// Signature 计算公众号回调签名：sha1(sort(token, timestamp, nonce) 拼接)。
// 明文/兼容模式的 GET 验签与 POST 消息校验都用它。
func Signature(token, timestamp, nonce string) string {
	arr := []string{token, timestamp, nonce}
	sort.Strings(arr)
	h := sha1.New()
	for _, s := range arr {
		_, _ = h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Verify 比对签名是否匹配。
func Verify(token, timestamp, nonce, signature string) bool {
	return Signature(token, timestamp, nonce) == signature
}

// MsgSignature 安全模式的消息签名：sha1(sort(token, timestamp, nonce, encrypt))。
// 安全模式回调校验与加密回复用。
func MsgSignature(token, timestamp, nonce, encrypt string) string {
	arr := []string{token, timestamp, nonce, encrypt}
	sort.Strings(arr)
	h := sha1.New()
	for _, s := range arr {
		_, _ = h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyMsg 校验安全模式回调的 msg_signature。
func VerifyMsg(token, timestamp, nonce, encrypt, msgSignature string) bool {
	return MsgSignature(token, timestamp, nonce, encrypt) == msgSignature
}
