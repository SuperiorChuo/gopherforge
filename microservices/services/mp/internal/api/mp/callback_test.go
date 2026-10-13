package api

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxcrypto"
	"gorm.io/gorm"
)

func newTestServer(t *testing.T) (*Server, *gin.Engine, *store.MpAccount) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	acc := &store.MpAccount{
		TenantID: 1, AppID: "wxappid001", Name: "测试号",
		Type: "service", GhID: "gh_test001", Token: "smoketoken",
		EncryptMode: "plain", Status: "active",
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	srv := &Server{Store: st, Secret: "test-jwt-secret"}
	r := gin.New()
	srv.RegisterRoutes(r)
	return srv, r, acc
}

func signedQuery(token string) (ts, nonce, sig string) {
	ts, nonce = "1700000000", "abcNONCE"
	sig = wxcrypto.Signature(token, ts, nonce)
	return
}

func TestCallbackVerifyEchoesOnValidSignature(t *testing.T) {
	_, r, acc := newTestServer(t)
	ts, nonce, sig := signedQuery(acc.Token)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=%s&timestamp=%s&nonce=%s&echostr=HELLO", acc.AppID, sig, ts, nonce)

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "HELLO" {
		t.Fatalf("verify: code=%d body=%q, want 200 HELLO", w.Code, w.Body.String())
	}
}

func TestCallbackVerifyRejectsBadSignature(t *testing.T) {
	_, r, acc := newTestServer(t)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=deadbeef&timestamp=1700000000&nonce=x&echostr=HELLO", acc.AppID)

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("bad signature: code=%d, want 403", w.Code)
	}
}

func TestCallbackVerifyUnknownAppID(t *testing.T) {
	_, r, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mp/callback/nosuchapp?signature=x&timestamp=1&nonce=y&echostr=Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown appid: code=%d, want 404", w.Code)
	}
}

func postMessage(r *gin.Engine, appID, token, body string) *httptest.ResponseRecorder {
	ts, nonce, sig := signedQuery(token)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=%s&timestamp=%s&nonce=%s", appID, sig, ts, nonce)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCallbackTextMessageReplies(t *testing.T) {
	srv, r, acc := newTestServer(t)
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_fan]]></FromUserName>` +
		`<CreateTime>1700000000</CreateTime>` +
		`<MsgType><![CDATA[text]]></MsgType>` +
		`<Content><![CDATA[你好]]></Content><MsgId>1001</MsgId></xml>`

	w := postMessage(r, acc.AppID, acc.Token, body)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	got := w.Body.String()
	if !bytes.Contains([]byte(got), []byte("收到：你好")) {
		t.Fatalf("reply missing echo: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte("<ToUserName><![CDATA[openid_fan]]>")) {
		t.Fatalf("reply ToUserName should be the fan openid: %s", got)
	}

	// 留档：in + out 两条
	var count int64
	srv.Store.DB().Model(&store.MpMessage{}).Count(&count)
	if count != 2 {
		t.Fatalf("message rows = %d, want 2 (in+out)", count)
	}
}

func TestInactiveAccountCallbackVerifiesButDoesNotProcess(t *testing.T) {
	srv, r, acc := newTestServer(t)
	if err := srv.Store.DB().Model(acc).Update("status", "inactive").Error; err != nil {
		t.Fatalf("disable account: %v", err)
	}
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_inactive]]></FromUserName>` +
		`<MsgType><![CDATA[text]]></MsgType>` +
		`<Content><![CDATA[不应回复]]></Content><MsgId>1002</MsgId></xml>`

	w := postMessage(r, acc.AppID, acc.Token, body)
	if w.Code != http.StatusOK || w.Body.String() != "success" {
		t.Fatalf("inactive callback: code=%d body=%q", w.Code, w.Body.String())
	}
	var count int64
	if err := srv.Store.DB().Model(&store.MpMessage{}).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("inactive callback archived %d messages, want 0", count)
	}
}

func TestCallbackDedupsRepeatedInbound(t *testing.T) {
	srv, r, acc := newTestServer(t)
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_fan]]></FromUserName>` +
		`<MsgType><![CDATA[text]]></MsgType>` +
		`<Content><![CDATA[重复]]></Content><MsgId>2002</MsgId></xml>`

	postMessage(r, acc.AppID, acc.Token, body)
	postMessage(r, acc.AppID, acc.Token, body) // 微信重推

	var inCount int64
	srv.Store.DB().Model(&store.MpMessage{}).Where("direction = 'in'").Count(&inCount)
	if inCount != 1 {
		t.Fatalf("inbound rows = %d, want 1 (deduped)", inCount)
	}
}

func TestCallbackReturnsRetryableErrorWhenInboundArchiveFails(t *testing.T) {
	srv, r, acc := newTestServer(t)
	if err := srv.Store.DB().Callback().Create().Before("gorm:create").Register("test:fail_inbound_archive", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "mp_messages" {
			tx.AddError(errors.New("forced inbound archive failure"))
		}
	}); err != nil {
		t.Fatalf("register failing callback: %v", err)
	}
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_archive_failure]]></FromUserName>` +
		`<MsgType><![CDATA[text]]></MsgType>` +
		`<Content><![CDATA[请重试]]></Content><MsgId>2003</MsgId></xml>`

	w := postMessage(r, acc.AppID, acc.Token, body)
	if w.Code != http.StatusInternalServerError || w.Body.String() != "message archive failed" {
		t.Fatalf("callback: code=%d body=%q, want retryable archive failure", w.Code, w.Body.String())
	}
}

func TestCallbackUsesWechatCreateTimeForReplyWindow(t *testing.T) {
	srv, r, acc := newTestServer(t)
	createdAt := time.Now().Add(-49 * time.Hour).Unix()
	body := fmt.Sprintf(`<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>`+
		`<FromUserName><![CDATA[openid_delayed]]></FromUserName>`+
		`<CreateTime>%d</CreateTime><MsgType><![CDATA[text]]></MsgType>`+
		`<Content><![CDATA[延迟消息]]></Content><MsgId>3003</MsgId></xml>`, createdAt)
	if w := postMessage(r, acc.AppID, acc.Token, body); w.Code != http.StatusOK {
		t.Fatalf("callback: code=%d body=%s", w.Code, w.Body.String())
	}
	var inbound store.MpMessage
	if err := srv.Store.DB().Where("account_id = ? AND direction = ? AND msg_id = ?", acc.ID, "in", "3003").First(&inbound).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	if delta := inbound.CreatedAt.Unix() - createdAt; delta < -1 || delta > 1 {
		t.Fatalf("stored created_at=%v, want unix %d", inbound.CreatedAt, createdAt)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID), `{"content":"不应发送"}`, 1)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "48 小时") {
		t.Fatalf("delayed callback reply: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCallbackSubscribeEventWelcomes(t *testing.T) {
	_, r, acc := newTestServer(t)
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_new]]></FromUserName>` +
		`<MsgType><![CDATA[event]]></MsgType>` +
		`<Event><![CDATA[subscribe]]></Event></xml>`

	w := postMessage(r, acc.AppID, acc.Token, body)
	if !bytes.Contains(w.Body.Bytes(), []byte("感谢关注")) {
		t.Fatalf("subscribe welcome missing: %s", w.Body.String())
	}
}

func TestCallbackUnsubscribeMarksFanWithoutErasingProfile(t *testing.T) {
	srv, r, acc := newTestServer(t)
	fan := &store.MpFan{
		TenantID: acc.TenantID, AccountID: acc.ID, OpenID: "openid_leaving",
		Nickname: "保留昵称", City: "上海", Subscribed: true,
	}
	if err := srv.Store.UpsertFan(fan); err != nil {
		t.Fatalf("seed fan: %v", err)
	}
	body := `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_leaving]]></FromUserName>` +
		`<MsgType><![CDATA[event]]></MsgType>` +
		`<Event><![CDATA[unsubscribe]]></Event></xml>`

	w := postMessage(r, acc.AppID, acc.Token, body)
	if w.Code != http.StatusOK || w.Body.String() != "success" {
		t.Fatalf("unsubscribe response: code=%d body=%q", w.Code, w.Body.String())
	}

	var got store.MpFan
	if err := srv.Store.DB().Where("account_id = ? AND open_id = ?", acc.ID, fan.OpenID).First(&got).Error; err != nil {
		t.Fatalf("load fan: %v", err)
	}
	if got.Subscribed {
		t.Fatal("fan should be marked unsubscribed")
	}
	if got.Nickname != fan.Nickname || got.City != fan.City {
		t.Fatalf("fan profile was overwritten: nickname=%q city=%q", got.Nickname, got.City)
	}
}
