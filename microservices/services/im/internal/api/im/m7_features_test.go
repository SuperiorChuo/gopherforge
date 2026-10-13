package api

// M7 测试：营业时间、访客历史、消息搜索。

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/im/internal/model"
)

func TestBusinessHoursWithin(t *testing.T) {
	mon10 := time.Date(2026, 7, 13, 10, 0, 0, 0, time.Local)  // 周一 10:00
	mon22 := time.Date(2026, 7, 13, 22, 30, 0, 0, time.Local) // 周一 22:30
	sun10 := time.Date(2026, 7, 19, 10, 0, 0, 0, time.Local)  // 周日 10:00

	cases := []struct {
		name string
		cfg  model.BusinessHoursConfig
		at   time.Time
		want bool
	}{
		{"disabled always within", model.BusinessHoursConfig{}, mon22, true},
		{"weekday working hour", model.BusinessHoursConfig{Enabled: true, Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "18:00"}, mon10, true},
		{"weekday after hours", model.BusinessHoursConfig{Enabled: true, Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "18:00"}, mon22, false},
		{"weekend excluded", model.BusinessHoursConfig{Enabled: true, Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "18:00"}, sun10, false},
		{"overnight shift in", model.BusinessHoursConfig{Enabled: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, Start: "22:00", End: "06:00"}, mon22, true},
		{"overnight shift out", model.BusinessHoursConfig{Enabled: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, Start: "22:00", End: "06:00"}, mon10, false},
		{"broken time falls open", model.BusinessHoursConfig{Enabled: true, Days: []int{1}, Start: "bad", End: "18:00"}, mon22, true},
	}
	for _, tc := range cases {
		if got := tc.cfg.WithinAt(tc.at); got != tc.want {
			t.Errorf("%s: WithinAt=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestOffHoursNotice(t *testing.T) {
	r, srv := newTestRouterWithServer(t)

	// 把 demo 站设为永不营业（周一 00:00-00:01 之外全休）
	bh := `{"enabled":true,"days":[1],"start":"00:00","end":"00:01","off_text":"打烊了，明早九点见"}`
	if err := srv.Store.DB().Exec("UPDATE im_sites SET business_hours = ? WHERE app_key = 'demo'", bh).Error; err != nil {
		t.Fatal(err)
	}

	token := guestSession(t, r, "guest-bh-1")
	publicID := createConversation(t, r, token)

	// 历史里应有 off_text 的系统提示
	w, env := do(t, r, http.MethodGet, "/api/v1/im/conversations/"+publicID+"/messages",
		nil, map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("messages status %d", w.Code)
	}
	var res struct {
		Messages []struct {
			MsgType string `json:"msg_type"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	decodeInto(t, env.Data, &res)
	found := false
	for _, m := range res.Messages {
		if m.MsgType == "event" && strings.Contains(m.Content, "打烊了") {
			found = true
		}
	}
	if !found {
		t.Fatalf("off-hours notice missing: %+v", res.Messages)
	}
}

func TestVisitorHistoryAndSearch(t *testing.T) {
	r, _ := newTestRouterWithServer(t)
	token := guestSession(t, r, "guest-hist-1")
	publicID := createConversation(t, r, token)
	guestAuth := map[string]string{"Authorization": "Bearer " + token}

	// 发一条含关键词的消息
	w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "发票开错了怎么办"}}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("send status %d", w.Code)
	}

	// 访客历史
	w, env := do(t, r, http.MethodGet, "/api/v1/im/agent/conversations/"+publicID+"/visitor-history", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("history status %d body %s", w.Code, w.Body.String())
	}
	var hist struct {
		List []struct {
			PublicID string `json:"public_id"`
		} `json:"list"`
	}
	decodeInto(t, env.Data, &hist)
	if len(hist.List) != 1 || hist.List[0].PublicID != publicID {
		t.Fatalf("history = %+v", hist.List)
	}

	// 消息搜索：命中
	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/messages/search?keyword=发票", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("search status %d body %s", w.Code, w.Body.String())
	}
	var sres struct {
		List []struct {
			ConversationPublicID string `json:"conversation_public_id"`
			Content              string `json:"content"`
		} `json:"list"`
	}
	decodeInto(t, env.Data, &sres)
	if len(sres.List) == 0 || sres.List[0].ConversationPublicID != publicID {
		t.Fatalf("search hits = %+v", sres.List)
	}

	// 空关键词 400
	w, _ = do(t, r, http.MethodGet, "/api/v1/im/agent/messages/search?keyword=", nil, agentHeaders)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty keyword status %d", w.Code)
	}
}
