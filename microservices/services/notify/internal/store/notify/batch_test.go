package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/notify/internal/model"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlCounter 是 gorm Logger，按语句形态计数（验证批量化后查询数不再随 N 增长）。
type sqlCounter struct {
	mu    sync.Mutex
	stmts []string
}

func (c *sqlCounter) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCounter) Info(context.Context, string, ...any)             {}
func (c *sqlCounter) Warn(context.Context, string, ...any)             {}
func (c *sqlCounter) Error(context.Context, string, ...any)            {}

func (c *sqlCounter) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.stmts = append(c.stmts, sql)
	c.mu.Unlock()
}

func (c *sqlCounter) reset() {
	c.mu.Lock()
	c.stmts = nil
	c.mu.Unlock()
}

// count 返回同时含全部 substr（大小写不敏感）的语句数。
func (c *sqlCounter) count(substrs ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, s := range c.stmts {
		low := strings.ToLower(s)
		all := true
		for _, sub := range substrs {
			if !strings.Contains(low, strings.ToLower(sub)) {
				all = false
				break
			}
		}
		if all {
			n++
		}
	}
	return n
}

func (c *sqlCounter) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.stmts)
}

// newCountingStore 建内存库 + identity owner API mock（Phase 2C 后不再直查共享 users 表），
// 并挂上语句计数器。tenant 1 有 users 1..n，tenant 2 有一个越权用户 9001。
func newCountingStore(t *testing.T, n int) (*Store, *sqlCounter) {
	t.Helper()
	counter := &sqlCounter{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: counter})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// identity /internal/users/contacts mock：tenant 1 有 users 1..n，tenant 2 有 9001。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID uint64   `json:"tenant_id"`
			IDs      []uint64 `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		contacts := map[string]any{}
		for _, id := range req.IDs {
			if req.TenantID == 1 && id >= 1 && id <= uint64(n) {
				contacts[fmt.Sprintf("%d", id)] = map[string]string{"email": "u@t1.example.com", "phone": ""}
			}
			if req.TenantID == 2 && id == 9001 {
				contacts["9001"] = map[string]string{"email": "x@t2.example.com", "phone": ""}
			}
			if req.TenantID == 7 && id == 77 {
				contacts["77"] = map[string]string{"email": "u@t7.example.com", "phone": ""}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
	}))
	st.SetIdentity(srv.URL, "test-internal-token")
	t.Cleanup(srv.Close)

	if err := db.Exec(`INSERT INTO notify_templates (tenant_id, code, title_tpl, body_tpl, link_tpl, channel, channels, enabled, created_at, updated_at)
		VALUES (1, 'batch.test', '标题 {{n}}', '正文 {{n}}', '/x', 'inbox', 'inbox', 1, ?, ?)`,
		time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("seed template: %v", err)
	}
	return st, counter
}

func userIDs(n int) []uint64 {
	ids := make([]uint64, 0, n)
	for i := 1; i <= n; i++ {
		ids = append(ids, uint64(i))
	}
	return ids
}

// 批量 fan-out 的查询次数不随收件人数增长：模板 1 次、租户校验 1 次、
// 去重 1 次、站内信 1 条批量 INSERT。
func TestSendBatchQueryCountIsConstant(t *testing.T) {
	const n = 50
	st, counter := newCountingStore(t, n)

	counter.reset()
	res, err := st.SendBatch(context.Background(), BatchSendInput{
		TenantID: 1, UserIDs: userIDs(n),
		TemplateCode: "batch.test",
		RefType:      "batch", RefID: "r1",
		Vars: map[string]string{"n": "1"},
	})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if res.Created != n {
		t.Fatalf("created = %d, want %d", res.Created, n)
	}

	if got := counter.count("from `notify_templates`"); got != 1 {
		t.Fatalf("template selects = %d, want 1 (was O(N)=%d)", got, n)
	}
	if got := counter.count("from `users`"); got != 0 {
		t.Fatalf("users selects = %d, want 0 (Phase 2C 已改走 identity owner API)", got)
	}
	if got := counter.count("from `notify_messages`"); got != 1 {
		t.Fatalf("dedupe selects = %d, want 1 (was O(N))", got)
	}
	if got := counter.count("insert", "notify_messages"); got != 1 {
		t.Fatalf("message inserts = %d, want 1 batch (was O(N))", got)
	}
	// 总量上限：原实现 50 用户 ≈ 250 条；批量后应为常数级个位数。
	if total := counter.total(); total > 8 {
		t.Fatalf("total statements = %d, want <= 8 for %d recipients", total, n)
	}

	var msgs int64
	st.DB().Model(&model.Message{}).Count(&msgs)
	if msgs != n {
		t.Fatalf("messages persisted = %d, want %d", msgs, n)
	}
}

// 批量化后跨租户收件人仍被拒：不落库、rejected 计数正确、同租户不受影响。
func TestSendBatchRejectsCrossTenant(t *testing.T) {
	st, _ := newCountingStore(t, 3)

	res, err := st.SendBatch(context.Background(), BatchSendInput{
		TenantID: 1, UserIDs: []uint64{1, 2, 9001, 9999},
		Title: "越权批量", Content: "c",
	})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if res.Created != 2 || res.Rejected != 2 {
		t.Fatalf("created=%d rejected=%d, want 2/2", res.Created, res.Rejected)
	}
	var msgs []model.Message
	st.DB().Order("user_id").Find(&msgs)
	if len(msgs) != 2 || msgs[0].UserID != 1 || msgs[1].UserID != 2 {
		t.Fatalf("messages = %+v, want only users 1,2", msgs)
	}
	for _, m := range msgs {
		if m.TenantID != 1 {
			t.Fatalf("message tenant = %d, want 1", m.TenantID)
		}
	}
	// 被拒用户不进外发收件人列表（不会拿到跨租户联系方式）。
	for _, rec := range res.Recipients {
		if rec.UserID == 9001 || rec.UserID == 9999 {
			t.Fatalf("rejected user %d leaked into recipients", rec.UserID)
		}
		if strings.Contains(rec.Contact.Email, "t2.example.com") {
			t.Fatalf("cross-tenant contact leaked: %+v", rec.Contact)
		}
	}
}

// 去重在批量路径仍生效：同 ref 二次投递整批 skipped，不产生第二条站内信。
func TestSendBatchDedupe(t *testing.T) {
	st, counter := newCountingStore(t, 5)
	in := BatchSendInput{
		TenantID: 1, UserIDs: userIDs(5),
		TemplateCode: "batch.test",
		RefType:      "batch", RefID: "same-ref",
		Vars: map[string]string{"n": "1"},
	}
	first, err := st.SendBatch(context.Background(), in)
	if err != nil {
		t.Fatalf("first SendBatch: %v", err)
	}
	if first.Created != 5 {
		t.Fatalf("first created = %d, want 5", first.Created)
	}

	counter.reset()
	second, err := st.SendBatch(context.Background(), in)
	if err != nil {
		t.Fatalf("second SendBatch: %v", err)
	}
	if second.Created != 0 || second.Skipped != 5 {
		t.Fatalf("second created=%d skipped=%d, want 0/5", second.Created, second.Skipped)
	}
	if got := counter.count("insert", "notify_messages"); got != 0 {
		t.Fatalf("duplicate run issued %d inserts, want 0", got)
	}
	var n int64
	st.DB().Model(&model.Message{}).Count(&n)
	if n != 5 {
		t.Fatalf("messages = %d, want 5 (no duplicates)", n)
	}
}

// 同一批里重复的 user_id 只受理一次，多出的计 skipped。
func TestSendBatchDeduplicatesWithinRequest(t *testing.T) {
	st, _ := newCountingStore(t, 2)
	res, err := st.SendBatch(context.Background(), BatchSendInput{
		TenantID: 1, UserIDs: []uint64{1, 1, 2, 0},
		Title: "重复", Content: "c",
	})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if res.Created != 2 || res.Skipped != 1 {
		t.Fatalf("created=%d skipped=%d, want 2/1", res.Created, res.Skipped)
	}
}

// 无 users 表时沿用软读放行（与 userInTenant 降级口径一致）。
func TestSendBatchSoftPassWithoutUsersTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	res, err := st.SendBatch(context.Background(), BatchSendInput{TenantID: 1, UserIDs: []uint64{7, 8}, Title: "t"})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if res.Created != 2 || res.Rejected != 0 {
		t.Fatalf("created=%d rejected=%d, want 2/0", res.Created, res.Rejected)
	}
}

// 模板 fallback 到 tenant 1 平台模板的语义保留（tenant 7 无自有模板）。
func TestSendBatchTemplateFallbackToPlatform(t *testing.T) {
	st, _ := newCountingStore(t, 1) // mock 含 user 77 → tenant 7（模板 fallback 场景）
	res, err := st.SendBatch(context.Background(), BatchSendInput{
		TenantID: 7, UserIDs: []uint64{77},
		TemplateCode: "batch.test",
		Vars:         map[string]string{"n": "9"},
	})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if res.Rendered.Title != "标题 9" {
		t.Fatalf("rendered title = %q, want platform template fallback", res.Rendered.Title)
	}
	if res.Created != 1 {
		t.Fatalf("created = %d, want 1", res.Created)
	}
}

// 空 title（无模板且无直发标题）仍报错，语义与单条 Send 一致。
func TestSendBatchRequiresTitle(t *testing.T) {
	st, _ := newCountingStore(t, 1)
	if _, err := st.SendBatch(context.Background(), BatchSendInput{TenantID: 1, UserIDs: []uint64{1}}); err == nil {
		t.Fatal("want title required error")
	}
	if _, err := st.SendBatch(context.Background(), BatchSendInput{TenantID: 1, Title: "t"}); err == nil {
		t.Fatal("want user_id required error")
	}
}

// 投递日志批量写：一条 INSERT 承载多行。
func TestSaveDeliveryLogsBatch(t *testing.T) {
	st, counter := newCountingStore(t, 1)
	logs := BuildDeliveryLogs(1, 1, 5, "batch.test", "batch", "r1", nil)
	if logs != nil {
		t.Fatalf("empty results should build no logs, got %+v", logs)
	}
	rows := make([]model.DeliveryLog, 0, 4)
	for i := 0; i < 4; i++ {
		rows = append(rows, model.DeliveryLog{TenantID: 1, UserID: uint64(i + 1), Channel: "email", Status: "sent"})
	}
	counter.reset()
	st.SaveDeliveryLogsBatch(rows)
	if got := counter.count("insert", "notify_delivery_logs"); got != 1 {
		t.Fatalf("delivery log inserts = %d, want 1", got)
	}
	var n int64
	st.DB().Model(&model.DeliveryLog{}).Count(&n)
	if n != 4 {
		t.Fatalf("delivery logs = %d, want 4", n)
	}
}
