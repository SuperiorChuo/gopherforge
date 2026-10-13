package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/model"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
	"gorm.io/gorm"
)

// blockingEmail 是可控速率的 email sender：每次 Send 阻塞 delay，用于验证
// 外发已离开请求路径（请求耗时不再随收件人数线性增长）。
type blockingEmail struct {
	delay time.Duration
	sent  atomic.Int64
	start chan struct{}
	once  sync.Once
}

func (e *blockingEmail) Enabled() bool { return true }
func (e *blockingEmail) Name() string  { return "test-smtp" }
func (e *blockingEmail) Send(ctx context.Context, to string, p channel.Payload) error {
	e.once.Do(func() {})
	if e.start != nil {
		select {
		case <-e.start:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.sent.Add(1)
	return nil
}

// newOutboxServer 建带 email 渠道的测试服务：users 1..n 属 tenant 1，
// 9001 属 tenant 2（越权用户）。
func newOutboxServer(t *testing.T, n int, email channel.EmailSender, opts OutboxOptions) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// sqlite :memory: 每条连接是独立库；worker 并发写会开新连接看到空库。
	// 固定单连接，让异步 worker 与请求线程共用同一个内存库。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	st, err := store.NewWithDB(db, "inbox,email")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// identity owner API mock（Phase 2C）：tenant 1 有 users 1..n，tenant 2 有 9001。
	idSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
	}))
	st.SetIdentity(idSrv.URL, "test-internal-token")
	t.Cleanup(idSrv.Close)

	disp := &channel.Dispatcher{Email: email}
	srv := &Server{Store: st, InternalToken: "tkn", Dispatcher: disp}
	srv.Outbox = NewOutbox(st, disp, opts)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Outbox.Shutdown(ctx)
	})
	r := gin.New()
	r.POST("/api/v1/notify/internal/send", srv.InternalSend)
	return srv, r
}

// 外发异步化：接口在 SMTP 还没跑完时就返回，站内信已落库，queued 计数等于
// 受理人数；graceful shutdown 后 in-flight 投递全部完成并写下投递日志。
func TestInternalSendQueuesExternalDeliveryAsync(t *testing.T) {
	const n = 20
	email := &blockingEmail{start: make(chan struct{}), delay: 5 * time.Millisecond}
	srv, r := newOutboxServer(t, n, email, OutboxOptions{Workers: 4, QueueSize: 128})

	body := `{"tenant_id":1,"user_ids":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20],` +
		`"title":"异步外发","content":"c","ref_type":"batch","ref_id":"async-1","channels":["inbox","email"]}`
	start := time.Now()
	w := postInternalSend(r, body)
	elapsed := time.Since(start)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	// 邮件发送被 start 闸门卡住，请求却已返回 → 外发不在请求路径上。
	if email.sent.Load() != 0 {
		t.Fatalf("email sent %d before request returned, want async", email.sent.Load())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("request took %s, external delivery still blocking", elapsed)
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"created":20`) || !strings.Contains(resp, `"queued":20`) {
		t.Fatalf("body = %s, want created:20 queued:20", resp)
	}
	if !strings.Contains(resp, `"dropped":0`) {
		t.Fatalf("body = %s, want dropped:0", resp)
	}
	// 站内信同步落库：用户刷新即可见。
	var msgs int64
	srv.Store.DB().Model(&model.Message{}).Count(&msgs)
	if msgs != n {
		t.Fatalf("messages = %d, want %d (inbox must be synchronous)", msgs, n)
	}

	// 放闸 + graceful shutdown：in-flight 投递跑完，日志落库。
	close(email.start)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Outbox.Shutdown(ctx); err != nil {
		t.Fatalf("outbox shutdown: %v", err)
	}
	if got := email.sent.Load(); got != n {
		t.Fatalf("emails sent = %d, want %d after shutdown drain", got, n)
	}
	var logs []model.DeliveryLog
	srv.Store.DB().Find(&logs)
	if len(logs) != n {
		t.Fatalf("delivery logs = %d, want %d", len(logs), n)
	}
	for _, l := range logs {
		if l.Status != "sent" || l.Channel != "email" {
			t.Fatalf("delivery log = %+v, want sent/email", l)
		}
	}
}

// 队列满不静默丢弃：dropped 计数进返回体，站内信仍全部落库。
func TestInternalSendReportsDroppedWhenQueueFull(t *testing.T) {
	email := &blockingEmail{start: make(chan struct{})}
	srv, r := newOutboxServer(t, 10, email, OutboxOptions{Workers: 1, QueueSize: 1})

	w := postInternalSend(r, `{"tenant_id":1,"user_ids":[1,2,3,4,5,6,7,8,9,10],`+
		`"title":"背压","content":"c","channels":["inbox","email"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"created":10`) {
		t.Fatalf("body = %s, want created:10", resp)
	}
	if srv.Outbox.Dropped() == 0 {
		t.Fatalf("body = %s, want some dropped deliveries counted (queue size 1)", resp)
	}
	if strings.Contains(resp, `"dropped":0`) {
		t.Fatalf("body = %s, want non-zero dropped field", resp)
	}
	var msgs int64
	srv.Store.DB().Model(&model.Message{}).Count(&msgs)
	if msgs != 10 {
		t.Fatalf("messages = %d, want 10 (inbox unaffected by outbox backpressure)", msgs)
	}
	close(email.start)
}

// 跨租户用户在异步路径下也不会被外发（不入队、不取联系方式）。
func TestInternalSendDoesNotQueueCrossTenant(t *testing.T) {
	email := &blockingEmail{}
	srv, r := newOutboxServer(t, 2, email, OutboxOptions{Workers: 2, QueueSize: 64})

	w := postInternalSend(r, `{"tenant_id":1,"user_ids":[1,9001],`+
		`"title":"越权外发","content":"c","channels":["inbox","email"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"created":1`) || !strings.Contains(resp, `"rejected":1`) ||
		!strings.Contains(resp, `"queued":1`) {
		t.Fatalf("body = %s, want created:1 rejected:1 queued:1", resp)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Outbox.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if got := email.sent.Load(); got != 1 {
		t.Fatalf("emails sent = %d, want 1 (cross-tenant recipient must not be mailed)", got)
	}
	var logs []model.DeliveryLog
	srv.Store.DB().Find(&logs)
	if len(logs) != 1 || logs[0].UserID != 1 {
		t.Fatalf("delivery logs = %+v, want single log for user 1", logs)
	}
}

// 纯站内信（无 email/sms）不入队，返回体 queued=0。
func TestInternalSendInboxOnlySkipsOutbox(t *testing.T) {
	email := &blockingEmail{}
	_, r := newOutboxServer(t, 2, email, OutboxOptions{Workers: 2, QueueSize: 8})

	w := postInternalSend(r, `{"tenant_id":1,"user_ids":[1,2],"title":"仅站内","content":"c","channels":["inbox"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	if resp := w.Body.String(); !strings.Contains(resp, `"queued":0`) {
		t.Fatalf("body = %s, want queued:0", resp)
	}
	if email.sent.Load() != 0 {
		t.Fatalf("email sent for inbox-only send")
	}
}

// Shutdown 后入队被拒（不 panic、不写关闭的 channel）。
func TestOutboxEnqueueAfterShutdown(t *testing.T) {
	email := &blockingEmail{}
	srv, _ := newOutboxServer(t, 1, email, OutboxOptions{Workers: 1, QueueSize: 4})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Outbox.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if srv.Outbox.Enqueue(outboxJob{userID: 1, channels: []string{"email"}}) {
		t.Fatal("enqueue after shutdown should fail")
	}
	// 幂等：二次 Shutdown 不 panic。
	if err := srv.Outbox.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	var nilbox *Outbox
	if nilbox.Enqueue(outboxJob{}) || nilbox.Dropped() != 0 {
		t.Fatal("nil outbox must be inert")
	}
	if err := nilbox.Shutdown(ctx); err != nil {
		t.Fatalf("nil shutdown: %v", err)
	}
}
