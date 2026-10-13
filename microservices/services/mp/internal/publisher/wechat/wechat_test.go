package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
	"gorm.io/gorm"
)

type testTokenCache struct{}

func (testTokenCache) Get(context.Context, string) (string, error)              { return "TEST_TOKEN", nil }
func (testTokenCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (testTokenCache) SetNX(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (testTokenCache) Del(context.Context, string) error { return nil }

type testSecretResolver struct{}

func (testSecretResolver) ResolveSecret(_ context.Context, account *store.MpAccount) (string, string, error) {
	return account.AppID, "test-secret", nil
}

func testClient(baseURL string) *wxclient.Client {
	tokens := wxclient.NewTokenManager(testTokenCache{}, testSecretResolver{})
	return wxclient.NewClient(tokens, wxclient.WithBaseURL(baseURL))
}

func TestAdapterCapabilities(t *testing.T) {
	a := New(nil, nil)
	if a.Channel() != content.ChannelWechat {
		t.Fatalf("channel = %q", a.Channel())
	}
	caps := a.Capabilities()
	if !caps.Publish || !caps.Export {
		t.Fatalf("wechat should support publish+export: %+v", caps)
	}
	// 满足 Publisher 接口
	var _ publisher.Publisher = a
}

func TestExportPackageStripsHTMLAndCollectsImages(t *testing.T) {
	a := New(nil, nil)
	post := &content.Post{
		Title:    "标题",
		BodyHTML: "<p>第一段</p><p>第二段</p>",
		CoverURL: "http://x/cover.png",
		Media:    content.MediaItemList{{Kind: "image", URL: "http://x/1.png", Name: "图1"}},
	}
	pkg, err := a.ExportPackage(context.Background(), nil, post)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(pkg.PlatformText, "标题") || !strings.Contains(pkg.PlatformText, "第一段") {
		t.Fatalf("platform text missing content: %q", pkg.PlatformText)
	}
	if strings.Contains(pkg.PlatformText, "<p>") {
		t.Fatalf("html not stripped: %q", pkg.PlatformText)
	}
	if len(pkg.Images) != 2 { // 封面 + 1 张
		t.Fatalf("images = %d, want 2", len(pkg.Images))
	}
}

func TestPublishRequiresWXClient(t *testing.T) {
	a := New(nil, nil) // 无 wx
	_, err := a.Publish(context.Background(), &content.ChannelAccount{RefID: 1}, &content.Post{CoverURL: "x"})
	if err == nil {
		t.Fatal("publish without wx client should error")
	}
}

func TestPublishRejectsInactiveAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	acc := &store.MpAccount{
		TenantID: 1, AppID: "wx-inactive-publisher", Name: "停用号",
		Type: "service", EncryptMode: "plain", Status: "inactive",
	}
	if err := st.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("create account: %v", err)
	}
	a := New(st, wxclient.NewClient(nil))
	_, err = a.Publish(
		context.Background(),
		&content.ChannelAccount{TenantID: 1, RefID: acc.ID},
		&content.Post{},
	)
	if err == nil || !strings.Contains(err.Error(), "已停用") {
		t.Fatalf("inactive publish returned %v", err)
	}
}

func TestPublishRewritesInlineImagesBeforeDraft(t *testing.T) {
	const pngData = "iVBORw0KGgo="
	var inlineUploads atomic.Int32
	draftContent := make(chan string, 1)
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/media/uploadimg":
			inlineUploads.Add(1)
			fmt.Fprint(w, `{"url":"https://mmbiz.qpic.cn/social-inline"}`)
		case "/cgi-bin/material/add_material":
			fmt.Fprint(w, `{"media_id":"SOCIAL-THUMB","url":"https://mmbiz.qpic.cn/cover"}`)
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
			fmt.Fprint(w, `{"media_id":"SOCIAL-DRAFT"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	acc := &store.MpAccount{
		TenantID: 1, AppID: "wx-social-publisher", Name: "社媒公众号",
		Type: "service", EncryptMode: "plain", Status: "active",
	}
	if err := st.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("create account: %v", err)
	}
	sourceHTML := `<p>正文<img src="data:image/png;base64,` + pngData + `"><img src="data:image/png;base64,` + pngData + `"></p>`
	post := &content.Post{
		Title: "统一内容", BodyHTML: sourceHTML,
		CoverURL: "data:image/png;base64," + pngData,
	}
	a := New(st, testClient(wxServer.URL))
	result, err := a.Publish(
		context.Background(),
		&content.ChannelAccount{TenantID: acc.TenantID, RefID: acc.ID},
		post,
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.ExternalID != "SOCIAL-DRAFT" || result.Async {
		t.Fatalf("publish result = %+v", result)
	}
	if got := inlineUploads.Load(); got != 1 {
		t.Fatalf("inline uploads = %d, want one deduplicated upload", got)
	}
	rewritten := <-draftContent
	if strings.Contains(rewritten, "data:image") || strings.Count(rewritten, "https://mmbiz.qpic.cn/social-inline") != 2 {
		t.Fatalf("draft content was not rewritten: %s", rewritten)
	}
	if post.BodyHTML != sourceHTML {
		t.Fatal("wechat publishing should preserve the platform-neutral source HTML")
	}
}
