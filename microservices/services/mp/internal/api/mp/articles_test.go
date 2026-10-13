package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxcrypto"
	"gorm.io/gorm"
)

func TestParseArticleJSON(t *testing.T) {
	// 标准 JSON
	title, digest, html := parseArticleJSON(`{"title":"标题A","digest":"摘要B","content":"<p>正文C</p>"}`)
	if title != "标题A" || digest != "摘要B" || html != "<p>正文C</p>" {
		t.Fatalf("parse standard: %q %q %q", title, digest, html)
	}
	// ```json 包裹
	title, _, html = parseArticleJSON("```json\n{\"title\":\"T\",\"content\":\"<p>X</p>\"}\n```")
	if title != "T" || html != "<p>X</p>" {
		t.Fatalf("parse fenced: %q %q", title, html)
	}
	// 非 JSON：整体当正文
	_, _, html = parseArticleJSON("就是一段纯文本")
	if html != "就是一段纯文本" {
		t.Fatalf("parse plain: %q", html)
	}
}

func TestArticleCRUD(t *testing.T) {
	srv, _, acc := newTestServer(t)
	art := &store.MpArticle{
		TenantID: 1, AccountID: acc.ID, Title: "测试文", ContentHTML: "<p>hi</p>",
		Status: "draft_ai", CreatedBy: 1,
	}
	if err := srv.Store.CreateArticle(art); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := srv.Store.GetArticle(art.ID, 1)
	if err != nil || got.Title != "测试文" {
		t.Fatalf("get: %v %+v", err, got)
	}
	// 租户隔离
	if _, err := srv.Store.GetArticle(art.ID, 999); err == nil {
		t.Fatal("other tenant should not read article")
	}
	list, total, _ := srv.Store.ListArticles(1, acc.ID, "draft_ai", 1, 10)
	if total != 1 || len(list) != 1 {
		t.Fatalf("list total=%d len=%d", total, len(list))
	}
}

func TestInactiveAccountCannotPublishArticle(t *testing.T) {
	srv, r, acc := newTestServer(t)
	srv.WX = testWXClient("http://127.0.0.1:1")
	if err := srv.Store.DB().Model(acc).Update("status", "inactive").Error; err != nil {
		t.Fatalf("disable account: %v", err)
	}
	art := &store.MpArticle{
		TenantID: 1, AccountID: acc.ID, Title: "停用账号文章",
		CoverURL: "data:image/png;base64,iVBORw0KGgo=", Status: "draft_ai",
	}
	if err := srv.Store.CreateArticle(art); err != nil {
		t.Fatalf("create article: %v", err)
	}
	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/articles/%d/publish", art.ID), "", 1)
	if w.Code != http.StatusConflict || !bytes.Contains(w.Body.Bytes(), []byte("已停用")) {
		t.Fatalf("inactive publish: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPublishArticleRewritesInlineImagesBeforeDraft(t *testing.T) {
	const (
		pngData  = "iVBORw0KGgo="
		jpegData = "/9j/"
	)
	var inlineUploads atomic.Int32
	var coverUploads atomic.Int32
	draftContent := make(chan string, 1)
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/media/uploadimg":
			n := inlineUploads.Add(1)
			fmt.Fprintf(w, `{"url":"https://mmbiz.qpic.cn/inline-%d"}`, n)
		case "/cgi-bin/material/add_material":
			coverUploads.Add(1)
			fmt.Fprint(w, `{"media_id":"THUMB-1","url":"https://mmbiz.qpic.cn/cover"}`)
		case "/cgi-bin/draft/add":
			var payload struct {
				Articles []struct {
					Content string `json:"content"`
				} `json:"articles"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode draft request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if len(payload.Articles) != 1 {
				t.Errorf("draft articles = %d, want 1", len(payload.Articles))
			} else {
				draftContent <- payload.Articles[0].Content
			}
			fmt.Fprint(w, `{"media_id":"DRAFT-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, r, acc := newTestServer(t)
	srv.WX = testWXClient(wxServer.URL)
	sourceHTML := `<p><img alt="一" src="data:image/png;base64,` + pngData + `" srcset="https://example.com/unsafe.png 2x"><img src=" data:image/png;base64,` + pngData + ` "></p>` +
		`<img src="data:image/jpeg;base64,` + jpegData + `"><img src="https://mmbiz.qpic.cn/already-hosted">`
	art := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Title: "正文图片",
		ContentHTML: sourceHTML, CoverURL: "data:image/png;base64," + pngData,
		Status: "draft_ai", CreatedBy: 1,
	}
	if err := srv.Store.CreateArticle(art); err != nil {
		t.Fatalf("create article: %v", err)
	}

	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/articles/%d/publish", art.ID), "", acc.TenantID)
	if w.Code != http.StatusOK {
		t.Fatalf("publish: code=%d body=%s", w.Code, w.Body.String())
	}
	if got := inlineUploads.Load(); got != 2 {
		t.Fatalf("inline uploads = %d, want 2 unique sources", got)
	}
	if got := coverUploads.Load(); got != 1 {
		t.Fatalf("cover uploads = %d, want 1", got)
	}
	rewritten := <-draftContent
	if strings.Contains(rewritten, "data:image") || strings.Contains(rewritten, "srcset") {
		t.Fatalf("draft content retained source image data: %s", rewritten)
	}
	if got := strings.Count(rewritten, "https://mmbiz.qpic.cn/inline-1"); got != 2 {
		t.Fatalf("deduplicated image occurrences = %d, want 2: %s", got, rewritten)
	}
	if !strings.Contains(rewritten, "https://mmbiz.qpic.cn/inline-2") || !strings.Contains(rewritten, "https://mmbiz.qpic.cn/already-hosted") {
		t.Fatalf("draft content missing rewritten or existing wechat image: %s", rewritten)
	}
	stored, err := srv.Store.GetArticle(art.ID, acc.TenantID)
	if err != nil {
		t.Fatalf("load stored article: %v", err)
	}
	if stored.ContentHTML != sourceHTML {
		t.Fatal("publishing should preserve the editable source HTML")
	}
	if stored.Status != "drafted" || stored.ThumbMediaID != "THUMB-1" || stored.DraftMediaID != "DRAFT-1" {
		t.Fatalf("stored publish state = %+v", stored)
	}
}

func TestPublishArticleInlineUploadFailureDoesNotCreateDraftOrMutateArticle(t *testing.T) {
	const (
		pngData  = "iVBORw0KGgo="
		jpegData = "/9j/"
	)
	var inlineUploads atomic.Int32
	var coverUploads atomic.Int32
	var draftAdds atomic.Int32
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/media/uploadimg":
			if inlineUploads.Add(1) == 1 {
				fmt.Fprint(w, `{"url":"https://mmbiz.qpic.cn/inline-1"}`)
				return
			}
			fmt.Fprint(w, `{"errcode":45009,"errmsg":"reach max api daily quota limit"}`)
		case "/cgi-bin/material/add_material":
			coverUploads.Add(1)
			fmt.Fprint(w, `{"media_id":"SHOULD-NOT-HAPPEN"}`)
		case "/cgi-bin/draft/add":
			draftAdds.Add(1)
			fmt.Fprint(w, `{"media_id":"SHOULD-NOT-HAPPEN"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, r, acc := newTestServer(t)
	srv.WX = testWXClient(wxServer.URL)
	sourceHTML := `<img src="data:image/png;base64,` + pngData + `"><img src="data:image/jpeg;base64,` + jpegData + `">`
	art := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Title: "上传失败",
		ContentHTML: sourceHTML, CoverURL: "data:image/png;base64," + pngData,
		Status: "draft_ai", CreatedBy: 1,
	}
	if err := srv.Store.CreateArticle(art); err != nil {
		t.Fatalf("create article: %v", err)
	}

	w := doAuthed(r, http.MethodPost, fmt.Sprintf("/api/v1/mp/articles/%d/publish", art.ID), "", acc.TenantID)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("publish: code=%d body=%s", w.Code, w.Body.String())
	}
	if got := inlineUploads.Load(); got != 2 {
		t.Fatalf("inline uploads = %d, want failure on second upload", got)
	}
	if coverUploads.Load() != 0 || draftAdds.Load() != 0 {
		t.Fatalf("failure should stop before cover/draft: cover=%d draft=%d", coverUploads.Load(), draftAdds.Load())
	}
	stored, err := srv.Store.GetArticle(art.ID, acc.TenantID)
	if err != nil {
		t.Fatalf("load stored article: %v", err)
	}
	if stored.ContentHTML != sourceHTML || stored.Status != "draft_ai" || stored.ThumbMediaID != "" || stored.DraftMediaID != "" || stored.PublishID != "" {
		t.Fatalf("failed publish mutated article: %+v", stored)
	}
}

// PUBLISHJOBFINISH 事件把 publishing 文章置 published。
func TestCallbackPublishResultUpdatesArticle(t *testing.T) {
	srv, r, acc := newTestServer(t)
	art := &store.MpArticle{
		TenantID: 1, AccountID: acc.ID, Title: "待发布", Status: "publishing",
		PublishID: "2100000001",
	}
	if err := srv.Store.CreateArticle(art); err != nil {
		t.Fatalf("create: %v", err)
	}

	body := publishResultXML("2100000001", "0")

	ts, nonce := "1700000000", "pubnonce"
	sig := wxcrypto.Signature(acc.Token, ts, nonce)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=%s&timestamp=%s&nonce=%s", acc.AppID, sig, ts, nonce)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}

	got, _ := srv.Store.GetArticle(art.ID, 1)
	if got.Status != "published" {
		t.Fatalf("status = %q, want published", got.Status)
	}
}

// 失败状态置 failed。
func TestCallbackPublishResultFailure(t *testing.T) {
	srv, r, acc := newTestServer(t)
	art := &store.MpArticle{TenantID: 1, AccountID: acc.ID, Status: "publishing", PublishID: "2100000002"}
	srv.Store.CreateArticle(art)

	body := publishResultXML("2100000002", "2")
	ts, nonce := "1700000000", "n2"
	sig := wxcrypto.Signature(acc.Token, ts, nonce)
	url := fmt.Sprintf("/api/v1/mp/callback/%s?signature=%s&timestamp=%s&nonce=%s", acc.AppID, sig, ts, nonce)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got, _ := srv.Store.GetArticle(art.ID, 1)
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestCallbackPublishResultIsScopedToVerifiedAccount(t *testing.T) {
	srv, r, acc := newTestServer(t)
	if err := content.Migrate(srv.Store.DB()); err != nil {
		t.Fatalf("migrate content: %v", err)
	}
	srv.Content = content.NewStore(srv.Store.DB())

	sameTenantAcc := &store.MpAccount{
		TenantID: 1, AppID: "wxappid002", Name: "同租户其他号", Type: "service",
		Token: "token-2", EncryptMode: "plain", Status: "active",
	}
	otherTenantAcc := &store.MpAccount{
		TenantID: 2, AppID: "wxappid003", Name: "其他租户号", Type: "service",
		Token: "token-3", EncryptMode: "plain", Status: "active",
	}
	if err := srv.Store.DB().Create(sameTenantAcc).Error; err != nil {
		t.Fatalf("seed same-tenant account: %v", err)
	}
	if err := srv.Store.DB().Create(otherTenantAcc).Error; err != nil {
		t.Fatalf("seed other-tenant account: %v", err)
	}

	const publishID = "2100000099"
	articles := []*store.MpArticle{
		{TenantID: 2, AccountID: otherTenantAcc.ID, Status: "publishing", PublishID: publishID},
		{TenantID: 1, AccountID: sameTenantAcc.ID, Status: "publishing", PublishID: publishID},
		{TenantID: 1, AccountID: acc.ID, Status: "publishing", PublishID: publishID},
	}
	for _, article := range articles {
		if err := srv.Store.CreateArticle(article); err != nil {
			t.Fatalf("seed article: %v", err)
		}
	}

	channelAccounts := []*content.ChannelAccount{
		{TenantID: 2, Channel: content.ChannelWechat, Name: "其他租户号", RefID: otherTenantAcc.ID},
		{TenantID: 1, Channel: content.ChannelWechat, Name: "同租户其他号", RefID: sameTenantAcc.ID},
		{TenantID: 1, Channel: content.ChannelWechat, Name: "已验签账号", RefID: acc.ID},
	}
	publications := make([]*content.PostPublication, 0, len(channelAccounts))
	for i, channelAccount := range channelAccounts {
		if err := srv.Content.UpsertChannelAccount(channelAccount); err != nil {
			t.Fatalf("seed channel account: %v", err)
		}
		publication := &content.PostPublication{
			TenantID: channelAccount.TenantID, PostID: uint64(i + 1),
			Channel: content.ChannelWechat, AccountID: channelAccount.ID,
			Status: "publishing", ExternalID: publishID,
		}
		if err := srv.Content.CreatePublication(publication); err != nil {
			t.Fatalf("seed publication: %v", err)
		}
		publications = append(publications, publication)
	}

	w := postMessage(r, acc.AppID, acc.Token, publishResultXML(publishID, "0"))
	if w.Code != http.StatusOK {
		t.Fatalf("callback: code=%d body=%s", w.Code, w.Body.String())
	}

	wantArticleStatus := []string{"publishing", "publishing", "published"}
	for i, article := range articles {
		got, err := srv.Store.GetArticle(article.ID, article.TenantID)
		if err != nil || got.Status != wantArticleStatus[i] {
			t.Fatalf("article[%d]: status=%q err=%v, want %q", i, got.Status, err, wantArticleStatus[i])
		}
	}
	wantPublicationStatus := []string{"publishing", "publishing", "published"}
	for i, publication := range publications {
		var got content.PostPublication
		if err := srv.Content.DB().First(&got, publication.ID).Error; err != nil || got.Status != wantPublicationStatus[i] {
			t.Fatalf("publication[%d]: status=%q err=%v, want %q", i, got.Status, err, wantPublicationStatus[i])
		}
	}
}

func TestInactiveAccountStillSettlesPublishResultWithoutArchiving(t *testing.T) {
	srv, r, acc := newTestServer(t)
	article := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Status: "publishing", PublishID: "2100000100",
	}
	if err := srv.Store.CreateArticle(article); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := srv.Store.DB().Model(acc).Update("status", "inactive").Error; err != nil {
		t.Fatalf("disable account: %v", err)
	}

	w := postMessage(r, acc.AppID, acc.Token, publishResultXML(article.PublishID, "0"))
	if w.Code != http.StatusOK || w.Body.String() != "success" {
		t.Fatalf("callback: code=%d body=%q", w.Code, w.Body.String())
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

func TestCallbackPublishResultPersistsUntilLocalRecordExists(t *testing.T) {
	srv, r, acc := newTestServer(t)
	if err := content.Migrate(srv.Store.DB()); err != nil {
		t.Fatalf("migrate content: %v", err)
	}
	srv.Content = content.NewStore(srv.Store.DB())

	const publishID = "2100000103"
	w := postMessage(r, acc.AppID, acc.Token, publishResultXML(publishID, "0"))
	if w.Code != http.StatusOK {
		t.Fatalf("early callback: code=%d body=%q", w.Code, w.Body.String())
	}
	var job store.MpPublishResultJob
	if err := srv.Store.DB().Where("publish_id = ?", publishID).First(&job).Error; err != nil {
		t.Fatalf("load persisted callback: %v", err)
	}
	if job.Status != "pending" {
		t.Fatalf("early callback status=%q, want pending", job.Status)
	}

	article := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Status: "publishing", PublishID: publishID,
	}
	if err := srv.Store.CreateArticle(article); err != nil {
		t.Fatalf("seed late article: %v", err)
	}
	channelAccount := &content.ChannelAccount{
		TenantID: acc.TenantID, Channel: content.ChannelWechat, Name: acc.Name, RefID: acc.ID,
	}
	if err := srv.Content.UpsertChannelAccount(channelAccount); err != nil {
		t.Fatalf("seed late channel account: %v", err)
	}
	publication := &content.PostPublication{
		TenantID: acc.TenantID, PostID: 1, Channel: content.ChannelWechat,
		AccountID: channelAccount.ID, Status: "publishing", ExternalID: publishID,
	}
	if err := srv.Content.CreatePublication(publication); err != nil {
		t.Fatalf("seed late publication: %v", err)
	}

	if err := srv.ReconcilePublishResultJobs(10); err != nil {
		t.Fatalf("reconcile persisted callback: %v", err)
	}
	gotArticle, err := srv.Store.GetArticle(article.ID, acc.TenantID)
	if err != nil || gotArticle.Status != "published" {
		t.Fatalf("article status=%q err=%v, want published", gotArticle.Status, err)
	}
	var gotPublication content.PostPublication
	if err := srv.Content.DB().First(&gotPublication, publication.ID).Error; err != nil || gotPublication.Status != "published" {
		t.Fatalf("publication status=%q err=%v, want published", gotPublication.Status, err)
	}
	if err := srv.Store.DB().First(&job, job.ID).Error; err != nil {
		t.Fatalf("reload callback job: %v", err)
	}
	if job.Status != "settled" || job.SettledAt == nil {
		t.Fatalf("callback job after reconcile=%+v, want settled", job)
	}
}

func TestCallbackPublishResultRejectsMissingStatus(t *testing.T) {
	srv, r, acc := newTestServer(t)
	w := postMessage(r, acc.AppID, acc.Token, publishResultXML("2100000104", ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("callback: code=%d body=%q, want 500", w.Code, w.Body.String())
	}
	var count int64
	if err := srv.Store.DB().Model(&store.MpPublishResultJob{}).Count(&count).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("malformed callback persisted %d jobs, want 0", count)
	}
}

func TestCallbackPublishResultReturnsRetryableErrorWhenUpdateFails(t *testing.T) {
	srv, r, acc := newTestServer(t)
	article := &store.MpArticle{
		TenantID: acc.TenantID, AccountID: acc.ID, Status: "publishing", PublishID: "2100000101",
	}
	if err := srv.Store.CreateArticle(article); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := srv.Store.DB().Callback().Update().Before("gorm:update").Register("test:fail_publish_update", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "mp_articles" {
			tx.AddError(errors.New("forced publish update failure"))
		}
	}); err != nil {
		t.Fatalf("register failing callback: %v", err)
	}

	w := postMessage(r, acc.AppID, acc.Token, publishResultXML(article.PublishID, "0"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("callback: code=%d body=%q, want 500", w.Code, w.Body.String())
	}
}

func publishResultXML(publishID, publishStatus string) string {
	return `<xml><ToUserName><![CDATA[gh_test001]]></ToUserName>` +
		`<FromUserName><![CDATA[openid_x]]></FromUserName>` +
		`<MsgType><![CDATA[event]]></MsgType>` +
		`<Event><![CDATA[PUBLISHJOBFINISH]]></Event>` +
		`<PublishEventInfo><publish_id>` + publishID + `</publish_id>` +
		`<publish_status>` + publishStatus + `</publish_status></PublishEventInfo></xml>`
}
