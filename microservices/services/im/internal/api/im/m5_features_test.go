package api

// M5 测试：快捷回复 CRUD、访客 CSAT 评分、访客卡片与备注。

import (
	"encoding/json"
	"net/http"
	"testing"
)

var agentHeaders = map[string]string{"X-Auth-User-ID": "9", "X-Auth-Username": "agent9", "X-Auth-Permissions": "*"}

func TestQuickReplyCRUD(t *testing.T) {
	r := newTestRouter(t)

	// 创建
	w, env := do(t, r, http.MethodPost, "/api/v1/im/agent/quick-replies", map[string]any{
		"title": "问候", "content": "您好，很高兴为您服务！",
	}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d body %s", w.Code, w.Body.String())
	}
	var qr struct {
		ID    uint64 `json:"id"`
		Title string `json:"title"`
	}
	decodeInto(t, env.Data, &qr)
	if qr.ID == 0 || qr.Title != "问候" {
		t.Fatalf("unexpected quick reply: %+v", qr)
	}

	// 空白内容被拒绝
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/agent/quick-replies", map[string]any{
		"title": "空的", "content": "   ",
	}, agentHeaders)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("blank content status %d", w.Code)
	}

	// 更新
	w, env = do(t, r, http.MethodPut, "/api/v1/im/agent/quick-replies/1", map[string]any{
		"content": "您好，请问有什么可以帮您？", "sort": 5,
	}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("update status %d body %s", w.Code, w.Body.String())
	}
	var updated struct {
		Content string `json:"content"`
		Sort    int    `json:"sort"`
	}
	decodeInto(t, env.Data, &updated)
	if updated.Sort != 5 || updated.Content != "您好，请问有什么可以帮您？" {
		t.Fatalf("unexpected update: %+v", updated)
	}

	// 列表
	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/quick-replies", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d", w.Code)
	}
	var list struct {
		List []json.RawMessage `json:"list"`
	}
	decodeInto(t, env.Data, &list)
	if len(list.List) != 1 {
		t.Fatalf("expect 1 quick reply, got %d", len(list.List))
	}

	// 删除，随后 404
	w, _ = do(t, r, http.MethodDelete, "/api/v1/im/agent/quick-replies/1", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status %d", w.Code)
	}
	w, _ = do(t, r, http.MethodDelete, "/api/v1/im/agent/quick-replies/1", nil, agentHeaders)
	if w.Code != http.StatusNotFound {
		t.Fatalf("re-delete status %d", w.Code)
	}
}

func TestVisitorRating(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "guest-rating-1")
	publicID := createConversation(t, r, token)
	guestAuth := map[string]string{"Authorization": "Bearer " + token}

	// 尚未关闭 → 拒绝
	w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 5}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("rate open conversation status %d", w.Code)
	}

	// 客服关闭
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/agent/conversations/"+publicID+"/close",
		map[string]any{"reason": "agent"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("close status %d body %s", w.Code, w.Body.String())
	}

	// 无效评分
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 9}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("score 9 status %d", w.Code)
	}

	// 正常路径
	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 5, "comment": "解决得很快"}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("rate status %d body %s", w.Code, w.Body.String())
	}
	var rated struct {
		Rated bool  `json:"rated"`
		Score int16 `json:"score"`
	}
	decodeInto(t, env.Data, &rated)
	if !rated.Rated || rated.Score != 5 {
		t.Fatalf("unexpected rating resp: %+v", rated)
	}

	// 二次评分被拒绝
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 1}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("re-rate status %d", w.Code)
	}

	// 其他访客不能评分
	otherToken := guestSession(t, r, "guest-rating-2")
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 1}, map[string]string{"Authorization": "Bearer " + otherToken})
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-visitor rate status %d", w.Code)
	}

	// 已关闭范围展示会话及其评分
	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/conversations?scope=closed", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("closed list status %d", w.Code)
	}
	var closedList struct {
		List []struct {
			PublicID    string `json:"public_id"`
			RatingScore *int16 `json:"rating_score"`
		} `json:"list"`
	}
	decodeInto(t, env.Data, &closedList)
	found := false
	for _, item := range closedList.List {
		if item.PublicID == publicID {
			found = true
			if item.RatingScore == nil || *item.RatingScore != 5 {
				t.Fatalf("closed list rating = %v", item.RatingScore)
			}
		}
	}
	if !found {
		t.Fatal("closed conversation missing from closed scope")
	}
}

func TestAgentVisitorCardAndNote(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "guest-card-1")
	publicID := createConversation(t, r, token)

	// 访客卡片包含会话数量
	w, env := do(t, r, http.MethodGet, "/api/v1/im/agent/conversations/"+publicID+"/visitor", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("get visitor status %d body %s", w.Code, w.Body.String())
	}
	var card struct {
		Visitor struct {
			ID   uint64 `json:"id"`
			Note string `json:"note"`
		} `json:"visitor"`
		ConversationCount int64 `json:"conversation_count"`
	}
	decodeInto(t, env.Data, &card)
	if card.Visitor.ID == 0 || card.ConversationCount != 1 {
		t.Fatalf("unexpected card: %+v", card)
	}

	// 写入备注，读回验证
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/agent/conversations/"+publicID+"/visitor-note",
		map[string]any{"note": "VIP 客户，优先响应"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("set note status %d body %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/conversations/"+publicID+"/visitor", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("re-get visitor status %d", w.Code)
	}
	decodeInto(t, env.Data, &card)
	if card.Visitor.Note != "VIP 客户，优先响应" {
		t.Fatalf("note not persisted: %q", card.Visitor.Note)
	}
}

func TestVisitorSourceAndPhone(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "guest-source-phone")
	guestAuth := map[string]string{"Authorization": "Bearer " + token}

	// 使用页面来源创建会话
	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations", map[string]any{
		"channel": "web_widget",
		"context": map[string]any{
			"page_url":   "https://shop.example.com/product/42",
			"page_title": "示例商品",
			"referrer":   "https://www.google.com/",
			"client_ip":  "1.2.3.4", // 必须被服务器覆盖
		},
	}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("create conv status %d body %s", w.Code, w.Body.String())
	}
	var conv struct {
		PublicID string `json:"public_id"`
		Context  string `json:"context"`
	}
	decodeInto(t, env.Data, &conv)
	if conv.PublicID == "" {
		t.Fatal("missing public_id")
	}
	var ctx map[string]any
	if err := json.Unmarshal([]byte(conv.Context), &ctx); err != nil {
		t.Fatalf("context json: %v raw=%s", err, conv.Context)
	}
	if ctx["page_url"] != "https://shop.example.com/product/42" {
		t.Fatalf("page_url = %v", ctx["page_url"])
	}
	if ctx["referrer"] != "https://www.google.com/" {
		t.Fatalf("referrer = %v", ctx["referrer"])
	}
	if ctx["client_ip"] == "1.2.3.4" {
		t.Fatal("client_ip should be server-authoritative, not client spoof")
	}
	if _, ok := ctx["client_ip"]; !ok {
		t.Fatal("client_ip missing from context")
	}

	// 访客留下电话
	w, env = do(t, r, http.MethodPut, "/api/v1/im/visitor/profile", map[string]any{
		"display_name": "张三",
		"phone":        "138-1234-5678",
	}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("profile status %d body %s", w.Code, w.Body.String())
	}
	var profile struct {
		Visitor struct {
			DisplayName string `json:"display_name"`
			Phone       string `json:"phone"`
		} `json:"visitor"`
	}
	decodeInto(t, env.Data, &profile)
	if profile.Visitor.DisplayName != "张三" || profile.Visitor.Phone != "13812345678" {
		t.Fatalf("profile = %+v", profile.Visitor)
	}

	// 客服卡片展示上下文与电话
	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/conversations/"+conv.PublicID+"/visitor", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("visitor card status %d body %s", w.Code, w.Body.String())
	}
	var card struct {
		Visitor struct {
			Phone       string `json:"phone"`
			DisplayName string `json:"display_name"`
		} `json:"visitor"`
		Context map[string]any `json:"context"`
	}
	decodeInto(t, env.Data, &card)
	if card.Visitor.Phone != "13812345678" || card.Visitor.DisplayName != "张三" {
		t.Fatalf("card visitor = %+v", card.Visitor)
	}
	if card.Context["page_title"] != "示例商品" {
		t.Fatalf("card context = %+v", card.Context)
	}

	// 客服可更新电话
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/agent/conversations/"+conv.PublicID+"/visitor-contact",
		map[string]any{"phone": "13900001111"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("agent contact status %d body %s", w.Code, w.Body.String())
	}

	// 客服可保存邮箱与微信
	w, env = do(t, r, http.MethodPut, "/api/v1/im/agent/conversations/"+conv.PublicID+"/visitor-contact",
		map[string]any{"email": "User@Example.COM", "wechat": "My_WeChat-01"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("email/wechat status %d body %s", w.Code, w.Body.String())
	}
	var contact struct {
		Visitor struct {
			Email  string `json:"email"`
			Wechat string `json:"wechat"`
		} `json:"visitor"`
	}
	decodeInto(t, env.Data, &contact)
	if contact.Visitor.Email != "user@example.com" || contact.Visitor.Wechat != "My_WeChat-01" {
		t.Fatalf("email/wechat = %+v", contact.Visitor)
	}

	// 无效电话被拒绝
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/visitor/profile",
		map[string]any{"phone": "abc"}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad phone status %d", w.Code)
	}

	// 无效邮箱被拒绝
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/visitor/profile",
		map[string]any{"email": "not-an-email"}, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad email status %d", w.Code)
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"13812345678", "13812345678"},
		{"138-1234-5678", "13812345678"},
		{"+86 138 0000 1111", "+8613800001111"},
		{"123", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizePhone(tc.in); got != tc.want {
			t.Fatalf("normalizePhone(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
