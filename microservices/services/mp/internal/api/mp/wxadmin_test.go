package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
)

type fixedTokenCache struct{}

func (fixedTokenCache) Get(context.Context, string) (string, error)              { return "TEST_TOKEN", nil }
func (fixedTokenCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (fixedTokenCache) SetNX(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (fixedTokenCache) Del(context.Context, string) error { return nil }

type fixedSecretResolver struct{}

func (fixedSecretResolver) ResolveSecret(_ context.Context, account *store.MpAccount) (string, string, error) {
	return account.AppID, "test-secret", nil
}

func testWXClient(baseURL string) *wxclient.Client {
	tokens := wxclient.NewTokenManager(fixedTokenCache{}, fixedSecretResolver{})
	return wxclient.NewClient(tokens, wxclient.WithBaseURL(baseURL))
}

func TestMenuDraftSaveReadAndAtomicUpsert(t *testing.T) {
	srv, r := newAdminServer(t)
	accountID := createAccount(t, r, "wxMENU", "菜单号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID)

	body := `{"buttons":[{"type":"click","name":"服务","key":"SERVICE"}]}`
	w := doAuthed(r, http.MethodPut, path, body, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("save draft: code=%d body=%s", w.Code, w.Body.String())
	}
	e := decodeEnvelope(t, w)
	var saved menuResponse
	if err := json.Unmarshal(e.Data, &saved); err != nil {
		t.Fatalf("decode saved menu: %v", err)
	}
	if saved.Published || len(saved.Buttons) == 0 {
		t.Fatalf("saved menu = %+v", saved)
	}

	body = `{"buttons":[{"type":"view","name":"官网","url":"https://example.com"}]}`
	w = doAuthed(r, http.MethodPut, path, body, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("update draft: code=%d body=%s", w.Code, w.Body.String())
	}
	var count int64
	if err := srv.Store.DB().Model(&store.MpMenu{}).Where("tenant_id = ? AND account_id = ?", 1, accountID).Count(&count).Error; err != nil {
		t.Fatalf("count menu rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("menu rows = %d, want 1", count)
	}

	w = doAuthed(r, http.MethodGet, path, "", 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "https://example.com") {
		t.Fatalf("get draft: code=%d body=%s", w.Code, w.Body.String())
	}
	w = doAuthed(r, http.MethodGet, path, "", 2)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetMenuTreatsLegacyEmptyTombstoneAsEmptyDraft(t *testing.T) {
	srv, r := newAdminServer(t)
	accountID := createAccount(t, r, "wxLEGACYMENU", "旧菜单号", 1)
	if err := srv.Store.DB().Create(&store.MpMenu{
		TenantID: 1, AccountID: accountID, Buttons: "", Published: false,
	}).Error; err != nil {
		t.Fatalf("seed legacy menu: %v", err)
	}
	w := doAuthed(r, http.MethodGet, fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID), "", 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"buttons":[]`) {
		t.Fatalf("legacy menu: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMenuRejectsUnknownTopLevelFieldAndInvalidLegacyShape(t *testing.T) {
	srv, r := newAdminServer(t)
	accountID := createAccount(t, r, "wxSTRICTMENU", "严格菜单号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID)
	w := doAuthed(r, http.MethodPut, path, `{"buttons":[{"type":"click","name":"入口","key":"K"}],"extra":true}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown top-level field: code=%d body=%s", w.Code, w.Body.String())
	}
	if err := srv.Store.UpsertMenu(1, accountID, `{}`, false); err != nil {
		t.Fatalf("seed invalid legacy menu: %v", err)
	}
	w = doAuthed(r, http.MethodGet, path, "", 1)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "本地菜单草稿格式无效") {
		t.Fatalf("invalid legacy menu: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMenuPublishAndDelete(t *testing.T) {
	var createBody string
	var deleteCalls int
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/menu/create":
			body, _ := io.ReadAll(r.Body)
			createBody = string(body)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		case "/cgi-bin/menu/delete":
			deleteCalls++
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxPUB", "发布号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID)
	body := `{"buttons":[{"type":"click","name":"客服","key":"HELP"}]}`
	if w := doAuthed(r, http.MethodPut, path, body, 1); w.Code != http.StatusOK {
		t.Fatalf("save: code=%d body=%s", w.Code, w.Body.String())
	}
	w := doAuthed(r, http.MethodPost, path+"/publish", `{}`, 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"published":true`) {
		t.Fatalf("publish: code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(createBody, `"button":[{"type":"click","name":"客服","key":"HELP"}]`) {
		t.Fatalf("wechat create body = %s", createBody)
	}

	if err := srv.Store.DB().Model(&store.MpAccount{}).Where("id = ?", accountID).Update("status", "inactive").Error; err != nil {
		t.Fatalf("disable account before cleanup: %v", err)
	}
	w = doAuthed(r, http.MethodDelete, path, "", 1)
	if w.Code != http.StatusOK || deleteCalls != 1 {
		t.Fatalf("delete: code=%d calls=%d body=%s", w.Code, deleteCalls, w.Body.String())
	}
	var count int64
	_ = srv.Store.DB().Model(&store.MpMenu{}).Where("account_id = ?", accountID).Count(&count).Error
	if count != 0 {
		t.Fatalf("local menu rows after delete = %d", count)
	}
}

func TestMenuPublishNormalizesLegacyDraftWithoutFalseCASConflict(t *testing.T) {
	var createBody string
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		createBody = string(body)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxLEGACYCAS", "历史草稿号", 1)
	rawDraft := "[ { \"key\" : \"K\", \"name\" : \"入口\", \"type\" : \"click\" } ]"
	if err := srv.Store.UpsertMenu(1, accountID, rawDraft, false); err != nil {
		t.Fatalf("seed legacy draft: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/accounts/%d/menu/publish", accountID), `{}`, 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"published":true`) {
		t.Fatalf("publish legacy draft: code=%d body=%s", w.Code, w.Body.String())
	}
	normalized := `[{"type":"click","name":"入口","key":"K"}]`
	if !strings.Contains(createBody, `"button":`+normalized) {
		t.Fatalf("wechat create body = %s", createBody)
	}
	menu, err := srv.Store.GetMenu(1, accountID)
	if err != nil || !menu.Published || menu.Buttons != normalized {
		t.Fatalf("normalized menu = %+v err=%v", menu, err)
	}
}

func TestMenuPublishFailureKeepsDraft(t *testing.T) {
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":45009,"errmsg":"api freq out of limit"}`))
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxFAIL", "失败号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID)
	body := `{"buttons":[{"type":"click","name":"客服","key":"HELP"}]}`
	if w := doAuthed(r, http.MethodPut, path, body, 1); w.Code != http.StatusOK {
		t.Fatalf("save: code=%d body=%s", w.Code, w.Body.String())
	}
	w := doAuthed(r, http.MethodPost, path+"/publish", `{}`, 1)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("publish failure: code=%d body=%s", w.Code, w.Body.String())
	}
	menu, err := srv.Store.GetMenu(1, accountID)
	if err != nil || menu.Published {
		t.Fatalf("draft after failure = %+v err=%v", menu, err)
	}
}

func TestMenuPublishDoesNotOverwriteConcurrentDraft(t *testing.T) {
	var srv *Server
	var accountID uint64
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := srv.Store.UpsertMenu(1, accountID, `[{"type":"click","name":"新版","key":"B"}]`, false); err != nil {
			t.Errorf("save concurrent draft: %v", err)
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer wxServer.Close()

	var r *gin.Engine
	srv, r = newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID = createAccount(t, r, "wxCASMENU", "并发菜单号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID)
	if w := doAuthed(r, http.MethodPut, path, `{"buttons":[{"type":"click","name":"旧版","key":"A"}]}`, 1); w.Code != http.StatusOK {
		t.Fatalf("save first draft: code=%d body=%s", w.Code, w.Body.String())
	}
	w := doAuthed(r, http.MethodPost, path+"/publish", `{}`, 1)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "本地已有更新草稿") {
		t.Fatalf("concurrent publish: code=%d body=%s", w.Code, w.Body.String())
	}
	menu, err := srv.Store.GetMenu(1, accountID)
	if err != nil || menu.Published || !strings.Contains(menu.Buttons, "新版") {
		t.Fatalf("concurrent draft after publish = %+v err=%v", menu, err)
	}
}

func TestDeleteMenuTreatsMissingRemoteAsSuccess(t *testing.T) {
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":46003,"errmsg":"menu no exist"}`))
	}))
	defer wxServer.Close()
	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxDELETEEMPTY", "空菜单号", 1)
	if err := srv.Store.UpsertMenu(1, accountID, `[{"type":"click","name":"草稿","key":"K"}]`, false); err != nil {
		t.Fatalf("seed local draft: %v", err)
	}
	w := doAuthed(r, http.MethodDelete, fmt.Sprintf("/api/v1/mp/accounts/%d/menu", accountID), "", 1)
	if w.Code != http.StatusOK {
		t.Fatalf("delete missing remote: code=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := srv.Store.GetMenu(1, accountID); err == nil {
		t.Fatal("local draft should be deleted")
	}
}

func TestReplyMessageUsesInboundTargetAndArchivesOutbound(t *testing.T) {
	var customerBody string
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		customerBody = string(body)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxREPLY", "客服号", 1)
	inbound := &store.MpMessage{
		TenantID: 1, AccountID: accountID, OpenID: "openid-safe", Direction: "in",
		MsgType: "text", Content: "你好", MsgID: "msg-reply", RawBody: "RAW-MUST-NOT-LEAK",
	}
	if err := srv.Store.LogMessage(inbound); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID), `{"content":"人工回复"}`, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("reply: code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(customerBody, `"touser":"openid-safe"`) || !strings.Contains(customerBody, `"content":"人工回复"`) {
		t.Fatalf("customer body = %s", customerBody)
	}

	w = doAuthed(r, http.MethodGet, "/api/v1/mp/messages", "", 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "人工回复") {
		t.Fatalf("list after reply: code=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "RAW-MUST-NOT-LEAK") || strings.Contains(w.Body.String(), "tenant_id") || strings.Contains(w.Body.String(), "raw_body") {
		t.Fatalf("message response leaked internal fields: %s", w.Body.String())
	}
}

func TestReplyMessageSendsImageAndArchivesMediaID(t *testing.T) {
	var customerBody string
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		customerBody = string(body)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxIMAGE", "图片客服号", 1)
	inbound := &store.MpMessage{
		TenantID: 1, AccountID: accountID, OpenID: "openid-image", Direction: "in",
		MsgType: "image", Content: "incoming-media", MsgID: "msg-image-reply",
	}
	if err := srv.Store.LogMessage(inbound); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID),
		`{"msg_type":"image","media_id":"MEDIA-OUT-1"}`, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("image reply: code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(customerBody, `"touser":"openid-image"`) ||
		!strings.Contains(customerBody, `"msgtype":"image"`) ||
		!strings.Contains(customerBody, `"media_id":"MEDIA-OUT-1"`) {
		t.Fatalf("customer image body = %s", customerBody)
	}

	list, total, err := srv.Store.ListMessages(1, accountID, "openid-image", "out", 1, 20)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("outbound archive: total=%d len=%d err=%v", total, len(list), err)
	}
	if list[0].MsgType != "image" || list[0].Content != "MEDIA-OUT-1" {
		t.Fatalf("outbound archive = %+v", list[0])
	}
}

func TestReplyMessageReturnsPartialSuccessWhenArchiveFails(t *testing.T) {
	var sends int
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sends++
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer wxServer.Close()
	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxARCHIVEFAIL", "留档失败号", 1)
	inbound := &store.MpMessage{
		TenantID: 1, AccountID: accountID, OpenID: "openid-archive", Direction: "in",
		MsgType: "text", Content: "你好", MsgID: "msg-archive-fail",
	}
	if err := srv.Store.LogMessage(inbound); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	if err := srv.Store.DB().Exec(`CREATE TRIGGER fail_outbound BEFORE INSERT ON mp_messages WHEN NEW.direction = 'out' BEGIN SELECT RAISE(FAIL, 'archive blocked'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID), `{"content":"只发送一次"}`, 1)
	if w.Code != http.StatusOK || sends != 1 || !strings.Contains(w.Body.String(), `"sent":true`) || !strings.Contains(w.Body.String(), `"archived":false`) {
		t.Fatalf("partial success: code=%d sends=%d body=%s", w.Code, sends, w.Body.String())
	}
}

func TestReplyMessageEnforcesTenantStatusAndWechatWindow(t *testing.T) {
	var sends int
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sends++
		_, _ = w.Write([]byte(`{"errcode":45015,"errmsg":"response out of time limit"}`))
	}))
	defer wxServer.Close()
	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxREPLYGUARDS", "回复门禁号", 1)
	inbound := &store.MpMessage{
		TenantID: 1, AccountID: accountID, OpenID: "openid-guards", Direction: "in",
		MsgType: "text", Content: "你好", MsgID: "msg-reply-guards",
	}
	if err := srv.Store.LogMessage(inbound); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	path := fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID)
	if w := doAuthed(r, http.MethodPost, path, `{"content":"跨租户"}`, 2); w.Code != http.StatusNotFound || sends != 0 {
		t.Fatalf("cross-tenant reply: code=%d sends=%d body=%s", w.Code, sends, w.Body.String())
	}
	if err := srv.Store.DB().Model(&store.MpAccount{}).Where("id = ?", accountID).Update("status", "inactive").Error; err != nil {
		t.Fatalf("disable account: %v", err)
	}
	if w := doAuthed(r, http.MethodPost, path, `{"content":"停用账号"}`, 1); w.Code != http.StatusConflict || sends != 0 {
		t.Fatalf("inactive reply: code=%d sends=%d body=%s", w.Code, sends, w.Body.String())
	}
	if err := srv.Store.DB().Model(&store.MpAccount{}).Where("id = ?", accountID).Update("status", "active").Error; err != nil {
		t.Fatalf("enable account: %v", err)
	}
	w := doAuthed(r, http.MethodPost, path, `{"content":"微信超窗"}`, 1)
	if w.Code != http.StatusConflict || sends != 1 || !strings.Contains(w.Body.String(), "48 小时") {
		t.Fatalf("wechat 45015: code=%d sends=%d body=%s", w.Code, sends, w.Body.String())
	}
}

func doAuthedImageUpload(r *gin.Engine, path, filename, contentType string, payload []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	partHeader.Set("Content-Type", contentType)
	part, _ := writer.CreatePart(partHeader)
	_, _ = part.Write(payload)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Auth-User-ID", "42")
	req.Header.Set("X-Auth-Username", "tester")
	req.Header.Set("X-Auth-Permissions", "*")
	req.Header.Set("X-Auth-Tenant-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUploadCustomerMediaValidatesAndUploadsImage(t *testing.T) {
	var gotType, gotFilename, gotPayload string
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/media/upload" {
			http.NotFound(w, r)
			return
		}
		gotType = r.URL.Query().Get("type")
		file, header, err := r.FormFile("media")
		if err != nil {
			t.Errorf("wechat form file: %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		defer file.Close()
		payload, _ := io.ReadAll(file)
		gotFilename = header.Filename
		gotPayload = string(payload)
		_, _ = w.Write([]byte(`{"type":"image","media_id":"MEDIA-UPLOAD-1","created_at":123}`))
	}))
	defer wxServer.Close()

	srv, r := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, r, "wxUPLOAD", "上传客服号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/customer-media", accountID)
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("test-image")...)
	w := doAuthedImageUpload(r, path, "reply.png", "image/png", png)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"media_id":"MEDIA-UPLOAD-1"`) {
		t.Fatalf("upload image: code=%d body=%s", w.Code, w.Body.String())
	}
	if gotType != "image" || gotFilename != "reply.png" || gotPayload != string(png) {
		t.Fatalf("wechat upload type=%q filename=%q payload=%q", gotType, gotFilename, gotPayload)
	}

	w = doAuthedImageUpload(r, path, "not-image.txt", "text/plain", []byte("plain text"))
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("invalid image: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestReplyMessageRejectsExpiredWindow(t *testing.T) {
	srv, r := newAdminServer(t)
	accountID := createAccount(t, r, "wxOLD", "过期号", 1)
	inbound := &store.MpMessage{
		TenantID: 1, AccountID: accountID, OpenID: "openid-old", Direction: "in",
		MsgType: "text", Content: "旧消息", MsgID: "msg-old", CreatedAt: time.Now().Add(-49 * time.Hour),
	}
	if err := srv.Store.LogMessage(inbound); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/messages/%d/reply", inbound.ID), `{"content":"回复"}`, 1)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "48 小时") {
		t.Fatalf("expired reply: code=%d body=%s", w.Code, w.Body.String())
	}
}
