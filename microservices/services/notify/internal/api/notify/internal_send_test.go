package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/notify/internal/model"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
	"gorm.io/gorm"
)

// newInternalSendServer 建含 identity owner API mock 的测试服务（Phase 2C）：
// user 10 → tenant 1，user 20 → tenant 2。
func newInternalSendServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// identity /internal/users/contacts mock。
	idSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID uint64   `json:"tenant_id"`
			IDs      []uint64 `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		contacts := map[string]any{}
		for _, id := range req.IDs {
			if id == 10 && req.TenantID == 1 {
				contacts["10"] = map[string]string{"email": "alice@t1.example.com", "phone": ""}
			}
			if id == 20 && req.TenantID == 2 {
				contacts["20"] = map[string]string{"email": "bob@t2.example.com", "phone": ""}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
	}))
	st.SetIdentity(idSrv.URL, "test-internal-token")
	t.Cleanup(idSrv.Close)

	srv := &Server{Store: st, InternalToken: "tkn"}
	r := gin.New()
	r.POST("/api/v1/notify/internal/send", srv.InternalSend)
	return srv, r
}

func postInternalSend(r http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notify/internal/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", "tkn")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 跨租户收件人被跳过（rejected 计数），同租户收件人正常投递，整批不失败。
func TestInternalSendSkipsCrossTenantUsers(t *testing.T) {
	srv, r := newInternalSendServer(t)

	w := postInternalSend(r, `{"tenant_id":1,"user_ids":[10,20],"title":"批量测试","content":"c"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"created":1`) || !strings.Contains(body, `"rejected":1`) {
		t.Fatalf("body = %s, want created:1 rejected:1", body)
	}

	var msgs []model.Message
	srv.Store.DB().Find(&msgs)
	if len(msgs) != 1 || msgs[0].UserID != 10 || msgs[0].TenantID != 1 {
		t.Fatalf("messages = %+v, want only user 10 @ tenant 1", msgs)
	}
}

// 单收件人跨租户：不落库、不 4xx 中断（rejected 计数返回给调用方）。
func TestInternalSendRejectsSingleCrossTenantUser(t *testing.T) {
	srv, r := newInternalSendServer(t)

	w := postInternalSend(r, `{"tenant_id":1,"user_id":20,"title":"越权测试","content":"c"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d body=%s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, `"created":0`) || !strings.Contains(body, `"rejected":1`) {
		t.Fatalf("body = %s, want created:0 rejected:1", body)
	}
	var n int64
	srv.Store.DB().Model(&model.Message{}).Count(&n)
	if n != 0 {
		t.Fatalf("messages = %d, want 0", n)
	}
}
