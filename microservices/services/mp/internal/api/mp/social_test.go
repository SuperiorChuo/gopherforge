package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"gorm.io/gorm"
)

// stubPublisher 假渠道 adapter：可配置 Publish 结果，验证内容中台编排。
type stubPublisher struct {
	channel     string
	publishResp publisher.PublishResult
	publishErr  error
	exportResp  publisher.ExportPackage
}

func (s *stubPublisher) Channel() publisher.Channel { return s.channel }
func (s *stubPublisher) Capabilities() publisher.Capabilities {
	return publisher.Capabilities{Publish: s.publishErr == nil, Export: true}
}
func (s *stubPublisher) Publish(_ context.Context, _ *content.ChannelAccount, _ *content.Post) (publisher.PublishResult, error) {
	return s.publishResp, s.publishErr
}
func (s *stubPublisher) ExportPackage(_ context.Context, _ *content.ChannelAccount, _ *content.Post) (publisher.ExportPackage, error) {
	return s.exportResp, nil
}
func (s *stubPublisher) QueryStatus(_ context.Context, _ *content.ChannelAccount, _ *content.PostPublication) (publisher.PublishStatus, error) {
	return publisher.PublishStatus{Done: true, Success: true}, nil
}

func newSocialServer(t *testing.T, pub *stubPublisher) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store migrate: %v", err)
	}
	if err := content.Migrate(db); err != nil {
		t.Fatalf("content migrate: %v", err)
	}
	reg := publisher.NewRegistry()
	if pub != nil {
		reg.Register(pub)
	}
	srv := &Server{Store: st, Secret: "test", Content: content.NewStore(db), Publishers: reg}
	r := gin.New()
	srv.RegisterRoutes(r)
	return srv, r
}

func doSocial(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "1")
	req.Header.Set("X-Auth-Tenant-ID", "1")
	req.Header.Set("X-Auth-Permissions", "*")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSocialPostCRUD(t *testing.T) {
	_, r := newSocialServer(t, nil)

	w := doSocial(r, http.MethodPost, "/api/v1/social/posts",
		`{"title":"多平台内容","body_html":"<p>hi</p>","platform":"wechat","tags":["a","b"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct{ Data content.Post }
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Data.ID == 0 || len(created.Data.Tags) != 2 {
		t.Fatalf("created post wrong: %+v", created.Data)
	}

	w = doSocial(r, http.MethodGet, "/api/v1/social/posts", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
}

func TestSocialPublishViaAdapter(t *testing.T) {
	pub := &stubPublisher{channel: "wechat", publishResp: publisher.PublishResult{ExternalID: "ext_1", ExternalURL: "http://x/1", Async: false}}
	srv, r := newSocialServer(t, pub)

	// 建内容 + 渠道账号
	post := &content.Post{TenantID: 1, Title: "t", BodyHTML: "<p>x</p>"}
	srv.Content.CreatePost(post)
	acc := &content.ChannelAccount{TenantID: 1, Channel: "wechat", Name: "号", RefID: 1}
	srv.Content.UpsertChannelAccount(acc)

	body := `{"channel":"wechat","account_id":` + itoa(acc.ID) + `}`
	w := doSocial(r, http.MethodPost, "/api/v1/social/posts/"+itoa(post.ID)+"/publish", body)
	if w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	var res struct{ Data content.PostPublication }
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Data.Status != "published" || res.Data.ExternalID != "ext_1" {
		t.Fatalf("publication wrong: %+v", res.Data)
	}

	// 发布记录已写库
	pubs, _ := srv.Content.ListPublications(1, post.ID)
	if len(pubs) != 1 {
		t.Fatalf("publications = %d, want 1", len(pubs))
	}
}

func TestSocialPublishUnknownChannel(t *testing.T) {
	srv, r := newSocialServer(t, nil)
	post := &content.Post{TenantID: 1, Title: "t"}
	srv.Content.CreatePost(post)
	w := doSocial(r, http.MethodPost, "/api/v1/social/posts/"+itoa(post.ID)+"/publish", `{"channel":"nope","account_id":1}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown channel: %d, want 400", w.Code)
	}
}

func TestSocialChannelsList(t *testing.T) {
	pub := &stubPublisher{channel: "wechat"}
	_, r := newSocialServer(t, pub)
	w := doSocial(r, http.MethodGet, "/api/v1/social/channels", "")
	if w.Code != http.StatusOK {
		t.Fatalf("channels: %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("wechat")) {
		t.Fatalf("channels should include wechat: %s", w.Body.String())
	}
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
