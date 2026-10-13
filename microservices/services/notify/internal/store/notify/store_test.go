package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/notify/internal/model"
	"gorm.io/gorm"
)

// newTestStore 建内存库 + identity owner API mock（Phase 2C 后不再直查共享 users 表）。
// mock 预置两个租户各一个用户：user 10 → tenant 1，user 20 → tenant 2。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// identity /internal/users/contacts mock：与生产语义一致（租户过滤）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID uint64   `json:"tenant_id"`
			IDs      []uint64 `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		contacts := map[string]any{}
		for _, id := range req.IDs {
			switch {
			case id == 10 && req.TenantID == 1:
				contacts["10"] = map[string]string{"email": "alice@t1.example.com", "phone": "13800000001"}
			case id == 20 && req.TenantID == 2:
				contacts["20"] = map[string]string{"email": "bob@t2.example.com", "phone": "13800000002"}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
	}))
	st.SetIdentity(srv.URL, "test-internal-token")
	t.Cleanup(srv.Close)
	return st
}

func TestSendRejectsCrossTenantUser(t *testing.T) {
	st := newTestStore(t)

	// user 20 属于 tenant 2；以 tenant 1 名义投递必须被拒且不落库。
	_, err := st.Send(context.Background(), SendInput{
		TenantID: 1, UserID: 20,
		Title: "跨租户测试", Content: "should not land",
	})
	if !errors.Is(err, ErrUserNotInTenant) {
		t.Fatalf("cross-tenant send err = %v, want ErrUserNotInTenant", err)
	}
	var n int64
	st.DB().Model(&model.Message{}).Count(&n)
	if n != 0 {
		t.Fatalf("messages = %d, want 0 (no cross-tenant inbox row)", n)
	}

	// 同租户正常投递不受影响。
	res, err := st.Send(context.Background(), SendInput{
		TenantID: 1, UserID: 10,
		Title: "同租户测试", Content: "ok",
	})
	if err != nil {
		t.Fatalf("same-tenant send: %v", err)
	}
	if res.Message == nil || res.Message.TenantID != 1 || res.Message.UserID != 10 {
		t.Fatalf("same-tenant message = %+v", res.Message)
	}
}

func TestSendRejectsUnknownUser(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Send(context.Background(), SendInput{TenantID: 1, UserID: 999, Title: "t"}); !errors.Is(err, ErrUserNotInTenant) {
		t.Fatalf("unknown user err = %v, want ErrUserNotInTenant", err)
	}
}

func TestSendSoftPassWithoutUsersTable(t *testing.T) {
	// 测试库/边缘环境无 users 表时沿用软读惯例放行（alerts_test 等依赖此行为）。
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := st.Send(context.Background(), SendInput{TenantID: 1, UserID: 1, Title: "t"}); err != nil {
		t.Fatalf("soft-pass send: %v", err)
	}
}

func TestGetUserContactTenantScoped(t *testing.T) {
	st := newTestStore(t)

	// 本租户用户：正常取到联系方式。
	c := st.GetUserContact(context.Background(), 1, 10)
	if c.Email != "alice@t1.example.com" || c.Phone != "13800000001" {
		t.Fatalf("own tenant contact = %+v", c)
	}

	// 跨租户：user 20 属于 tenant 2，以 tenant 1 查询必须拿不到 email/phone。
	c = st.GetUserContact(context.Background(), 1, 20)
	if c.Email != "" || c.Phone != "" {
		t.Fatalf("cross-tenant contact leaked: %+v", c)
	}

	// tenantID=0 回退租户 1（与 Send 同口径）。
	c = st.GetUserContact(context.Background(), 0, 10)
	if c.Email != "alice@t1.example.com" {
		t.Fatalf("default tenant contact = %+v", c)
	}
}
