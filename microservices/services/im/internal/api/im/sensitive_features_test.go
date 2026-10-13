package api

// 敏感词管理（控制台词库）测试：CRUD、打码/拦截、停用热生效、试敏接口。

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-admin-kit/services/im/internal/sensitive"
)

func TestSensitiveWordLifecycle(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.Sensitive = sensitive.NewCache(srv.Store)

	// 批量导入：两个打码词
	w, env := do(t, r, http.MethodPost, "/api/v1/im/admin/sensitive-words",
		map[string]any{"words": []string{"骗子", " 劣质 ", "骗子"}, "action": "mask", "tag": "辱骂"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d body %s", w.Code, w.Body.String())
	}
	var created struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
	}
	decodeInto(t, env.Data, &created)
	if created.Created != 2 {
		t.Fatalf("created = %+v", created)
	}

	// 重复导入 → 跳过
	w, env = do(t, r, http.MethodPost, "/api/v1/im/admin/sensitive-words",
		map[string]any{"word": "骗子"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("dup create status %d", w.Code)
	}
	decodeInto(t, env.Data, &created)
	if created.Created != 0 || created.Skipped != 1 {
		t.Fatalf("dup created = %+v", created)
	}

	// 拦截词
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/admin/sensitive-words",
		map[string]any{"word": "加微信", "action": "block", "tag": "引流"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("block create status %d", w.Code)
	}

	// 列表 + 筛选
	w, env = do(t, r, http.MethodGet, "/api/v1/im/admin/sensitive-words?action=block", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d", w.Code)
	}
	var listResp struct {
		Total int64 `json:"total"`
		List  []struct {
			ID     uint64 `json:"id"`
			Word   string `json:"word"`
			Action string `json:"action"`
		} `json:"list"`
	}
	decodeInto(t, env.Data, &listResp)
	if listResp.Total != 1 || listResp.List[0].Word != "加微信" {
		t.Fatalf("list = %+v", listResp)
	}
	blockID := listResp.List[0].ID

	// 访客发消息：打码词生效
	token := guestSession(t, r, "guest-sensitive-1")
	publicID := createConversation(t, r, token)
	guestAuth := map[string]string{"Authorization": "Bearer " + token}
	w, env = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "你就是骗子吧，卖劣质货"}}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("send status %d body %s", w.Code, w.Body.String())
	}
	var msg struct {
		Content string `json:"content"`
	}
	decodeInto(t, env.Data, &msg)
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &content); err != nil {
		t.Fatal(err)
	}
	if content.Text != "你就是**吧，卖**货" {
		t.Fatalf("masked text = %q", content.Text)
	}

	// 拦截词：拒发
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "加微信详聊"}}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("blocked send status %d body %s", w.Code, w.Body.String())
	}

	// 试敏接口
	w, env = do(t, r, http.MethodPost, "/api/v1/im/admin/sensitive-words/test",
		map[string]any{"text": "这个骗子让我加微信"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("test status %d", w.Code)
	}
	var testResp struct {
		MaskedText string          `json:"masked_text"`
		Blocked    bool            `json:"blocked"`
		Hits       []sensitive.Hit `json:"hits"`
	}
	decodeInto(t, env.Data, &testResp)
	if !testResp.Blocked || len(testResp.Hits) != 2 {
		t.Fatalf("test resp = %+v", testResp)
	}

	// 停用拦截词 → 热生效，可以发了
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/admin/sensitive-words/"+itoa(blockID),
		map[string]any{"status": 0}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("disable status %d body %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "加微信详聊"}}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("post-disable send status %d body %s", w.Code, w.Body.String())
	}

	// 删除
	w, _ = do(t, r, http.MethodDelete, "/api/v1/im/admin/sensitive-words/"+itoa(blockID), nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status %d", w.Code)
	}
	w, _ = do(t, r, http.MethodDelete, "/api/v1/im/admin/sensitive-words/"+itoa(blockID), nil, agentHeaders)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("re-delete status %d", w.Code)
	}
}

func TestSensitiveTenantIsolation(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.Sensitive = sensitive.NewCache(srv.Store)

	// 租户 2 的词不影响租户 1（会话/坐席默认租户 1）
	if _, _, err := srv.Store.CreateSensitiveWords(context.Background(), 2, 1, []string{"别家词"}, "block", ""); err != nil {
		t.Fatal(err)
	}
	token := guestSession(t, r, "guest-tenant-iso")
	publicID := createConversation(t, r, token)
	w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "别家词在此"}},
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("cross-tenant send status %d", w.Code)
	}
}

func TestSensitiveEnvFallbackStillMasks(t *testing.T) {
	// DB 词库与 env 兜底词叠加
	r, srv := newTestRouterWithServer(t)
	srv.Sensitive = sensitive.NewCache(srv.Store)
	srv.SensitiveWords = []string{"envword"}
	if _, _, err := srv.Store.CreateSensitiveWords(context.Background(), 1, 1, []string{"库词"}, "mask", ""); err != nil {
		t.Fatal(err)
	}
	token := guestSession(t, r, "guest-env-fallback")
	publicID := createConversation(t, r, token)
	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "库词 and ENVWORD"}},
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("send status %d", w.Code)
	}
	var msg struct {
		Content string `json:"content"`
	}
	decodeInto(t, env.Data, &msg)
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &content); err != nil {
		t.Fatal(err)
	}
	if content.Text != "** and *******" {
		t.Fatalf("masked text = %q", content.Text)
	}
}
