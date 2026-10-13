package api

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxcrypto"
	"github.com/go-admin-kit/services/mp/internal/wxmsg"
)

func genAESKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	full := base64.StdEncoding.EncodeToString(key)
	return full[:len(full)-1]
}

// 安全模式回调：加密入站消息 → 服务解密处理 → 返回加密回复 → 客户端解密校验。
func TestCallbackSafeModeRoundTrip(t *testing.T) {
	srv, r, acc := newTestServer(t)
	// 账号切安全模式，配 AESKey resolver。
	aesKey := genAESKey(t)
	acc.EncryptMode = "safe"
	srv.Store.DB().Save(acc)
	srv.AESKeyForAcct = func(a *store.MpAccount) string { return aesKey }

	cipher, err := wxcrypto.NewAESCipher(aesKey, acc.AppID)
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}

	// 构造明文入站消息并加密。
	plainMsg := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_safe]]></FromUserName>` +
		`<MsgType><![CDATA[text]]></MsgType>` +
		`<Content><![CDATA[安全你好]]></Content><MsgId>7001</MsgId></xml>`
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	encrypted, err := cipher.Encrypt([]byte(plainMsg), nonce[:])
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	ts, wnonce := "1700000000", "safenonce"
	msgSig := wxcrypto.MsgSignature(acc.Token, ts, wnonce, encrypted)
	body := fmt.Sprintf(`<xml><ToUserName><![CDATA[gh_test001]]></ToUserName><Encrypt><![CDATA[%s]]></Encrypt></xml>`, encrypted)

	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=x&timestamp=%s&nonce=%s&encrypt_type=aes&msg_signature=%s",
		acc.AppID, ts, wnonce, msgSig)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	// 回复应是加密信封，解析出 Encrypt 再解密。
	env, err := wxmsg.ParseEncrypted(w.Body.Bytes())
	if err != nil || env.Encrypt == "" {
		t.Fatalf("reply not an encrypted envelope: %s", w.Body.String())
	}
	replyPlain, err := cipher.Decrypt(env.Encrypt)
	if err != nil {
		t.Fatalf("decrypt reply: %v", err)
	}
	if !bytes.Contains(replyPlain, []byte("收到：安全你好")) {
		t.Fatalf("decrypted reply missing echo: %s", replyPlain)
	}
	if !bytes.Contains(replyPlain, []byte("openid_safe")) {
		t.Fatalf("reply ToUserName should be fan openid: %s", replyPlain)
	}
}

// 安全模式下 msg_signature 不对应拒绝。
func TestCallbackSafeModeRejectsBadMsgSignature(t *testing.T) {
	srv, r, acc := newTestServer(t)
	aesKey := genAESKey(t)
	acc.EncryptMode = "safe"
	srv.Store.DB().Save(acc)
	srv.AESKeyForAcct = func(a *store.MpAccount) string { return aesKey }

	body := `<xml><Encrypt><![CDATA[whatever]]></Encrypt></xml>`
	url := fmt.Sprintf("/api/v1/mp/callback/%s?timestamp=1&nonce=n&encrypt_type=aes&msg_signature=bad", acc.AppID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want 403", w.Code)
	}
}

func TestInactiveAccountSafeCallbackSettlesPublishResult(t *testing.T) {
	srv, r, acc := newTestServer(t)
	aesKey := genAESKey(t)
	acc.EncryptMode = "safe"
	acc.Status = "inactive"
	if err := srv.Store.DB().Save(acc).Error; err != nil {
		t.Fatalf("disable safe account: %v", err)
	}
	srv.AESKeyForAcct = func(a *store.MpAccount) string { return aesKey }
	article := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Status: "publishing", PublishID: "2100000102",
	}
	if err := srv.Store.CreateArticle(article); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	cipher, err := wxcrypto.NewAESCipher(aesKey, acc.AppID)
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	encrypted, err := cipher.Encrypt([]byte(publishResultXML(article.PublishID, "0")), random[:])
	if err != nil {
		t.Fatalf("encrypt publish result: %v", err)
	}
	ts, nonce := "1700000000", "inactive-safe"
	msgSig := wxcrypto.MsgSignature(acc.Token, ts, nonce, encrypted)
	body := fmt.Sprintf(`<xml><Encrypt><![CDATA[%s]]></Encrypt></xml>`, encrypted)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?timestamp=%s&nonce=%s&encrypt_type=aes&msg_signature=%s",
		acc.AppID, ts, nonce, msgSig)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "success" {
		t.Fatalf("inactive safe callback: code=%d body=%q", w.Code, w.Body.String())
	}
	got, err := srv.Store.GetArticle(article.ID, acc.TenantID)
	if err != nil || got.Status != "published" {
		t.Fatalf("article status=%q err=%v, want published", got.Status, err)
	}
	var messageCount int64
	if err := srv.Store.DB().Model(&store.MpMessage{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("inactive publish result archived %d messages, want 0", messageCount)
	}
}
