package content

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func TestPostCRUDAndTenantIsolation(t *testing.T) {
	s := newTestStore(t)
	p := &Post{
		TenantID: 1, Title: "秋日茶饮", BodyHTML: "<p>正文</p>", Summary: "摘要",
		Platform: ChannelWechat, CreatedBy: 7,
		Media: MediaItemList{{Kind: "image", URL: "http://x/1.png"}},
		Tags:  StringList{"养生", "茶"},
	}
	if err := s.CreatePost(p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetPost(p.ID, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "秋日茶饮" || len(got.Media) != 1 || got.Media[0].URL != "http://x/1.png" || len(got.Tags) != 2 {
		t.Fatalf("post fields/JSON roundtrip wrong: %+v", got)
	}
	if got.Status != "draft" {
		t.Fatalf("default status = %q, want draft", got.Status)
	}
	// 租户隔离
	if _, err := s.GetPost(p.ID, 999); err == nil {
		t.Fatal("other tenant should not read post")
	}
	list, total, _ := s.ListPosts(1, PostFilter{Platform: ChannelWechat})
	if total != 1 || len(list) != 1 {
		t.Fatalf("list total=%d len=%d", total, len(list))
	}
}

func TestPublicationLifecycle(t *testing.T) {
	s := newTestStore(t)
	post := &Post{TenantID: 1, Title: "t"}
	s.CreatePost(post)
	account := &ChannelAccount{TenantID: 1, Channel: ChannelWechat, Name: "公众号", RefID: 5}
	if err := s.UpsertChannelAccount(account); err != nil {
		t.Fatalf("create channel account: %v", err)
	}

	pub := &PostPublication{
		TenantID: 1, PostID: post.ID, Channel: ChannelWechat, AccountID: account.ID,
		Mode: "api", ExternalID: "pub_123",
	}
	if err := s.CreatePublication(pub); err != nil {
		t.Fatalf("create pub: %v", err)
	}
	if pub.Status != "pending" {
		t.Fatalf("default pub status = %q", pub.Status)
	}
	// 按外部 id 与已验签平台账号归属回写
	updated, err := s.UpdatePublicationStatusByExternalID(1, ChannelWechat, "pub_123", account.RefID, "published")
	if err != nil || !updated {
		t.Fatalf("update by external id: updated=%v err=%v", updated, err)
	}
	pubs, _ := s.ListPublications(1, post.ID)
	if len(pubs) != 1 || pubs[0].Status != "published" {
		t.Fatalf("list pubs: %+v", pubs)
	}
}

func TestChannelAccountUpsert(t *testing.T) {
	s := newTestStore(t)
	a := &ChannelAccount{TenantID: 1, Channel: ChannelWechat, Name: "公众号A", RefID: 3}
	if err := s.UpsertChannelAccount(a); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if a.ID == 0 || a.Status != "active" {
		t.Fatalf("after insert: %+v", a)
	}
	a.Name = "公众号A改名"
	if err := s.UpsertChannelAccount(a); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ := s.ListChannelAccounts(1, ChannelWechat)
	if len(list) != 1 || list[0].Name != "公众号A改名" {
		t.Fatalf("list: %+v", list)
	}
}
