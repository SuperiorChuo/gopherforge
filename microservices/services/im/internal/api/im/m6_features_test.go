package api

// M6 测试：统计、访客封禁、消息撤回、敏感词屏蔽、
// 排队位置。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestVisitorBanBlocksMessaging(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	token := guestSession(t, r, "guest-ban-1")
	publicID := createConversation(t, r, token)
	guestAuth := map[string]string{"Authorization": "Bearer " + token}

	// 通过客服接口封禁
	w, _ := do(t, r, http.MethodPut, "/api/v1/im/agent/conversations/"+publicID+"/visitor-ban",
		map[string]any{"banned": true}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("ban status %d body %s", w.Code, w.Body.String())
	}

	// 被封禁的访客不能发送
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "hi"}}, guestAuth)
	if w.Code != http.StatusForbidden {
		t.Fatalf("banned send status %d", w.Code)
	}

	// 关闭当前会话，被封禁的访客无法开启新会话
	if _, err := srv.Store.GetConversationByPublicID(publicID); err != nil {
		t.Fatal(err)
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/agent/conversations/"+publicID+"/close",
		map[string]any{"reason": "agent"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("close status %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations", map[string]any{}, guestAuth)
	if w.Code != http.StatusForbidden {
		t.Fatalf("banned create status %d", w.Code)
	}

	// 解封恢复
	w, _ = do(t, r, http.MethodPut, "/api/v1/im/agent/conversations/"+publicID+"/visitor-ban",
		map[string]any{"banned": false}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("unban status %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations", map[string]any{}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("post-unban create status %d body %s", w.Code, w.Body.String())
	}
}

func TestSensitiveWordMasking(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.SensitiveWords = []string{"辣鸡", "scam"}
	token := guestSession(t, r, "guest-mask-1")
	publicID := createConversation(t, r, token)

	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "你们真是辣鸡平台 SCAM 吗"}},
		map[string]string{"Authorization": "Bearer " + token})
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
	if content.Text != "你们真是**平台 **** 吗" {
		t.Fatalf("masked text = %q", content.Text)
	}
}

func TestMessageRecall(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	token := guestSession(t, r, "guest-recall-1")
	publicID := createConversation(t, r, token)
	guestAuth := map[string]string{"Authorization": "Bearer " + token}

	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "手滑发错了"}}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("send status %d", w.Code)
	}
	var sent struct {
		ID uint64 `json:"id"`
	}
	decodeInto(t, env.Data, &sent)

	// 客服不能撤回访客的消息
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages/"+itoa(sent.ID)+"/recall", nil, agentHeaders)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("cross-sender recall status %d", w.Code)
	}

	// 发送者在时间窗口内撤回
	w, env = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages/"+itoa(sent.ID)+"/recall", nil, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatalf("recall status %d body %s", w.Code, w.Body.String())
	}
	var recalled struct {
		MsgType string `json:"msg_type"`
		Content string `json:"content"`
	}
	decodeInto(t, env.Data, &recalled)
	if recalled.MsgType != "event" || recalled.Content == "" {
		t.Fatalf("recalled message: %+v", recalled)
	}

	// 窗口过期：将消息日期回拨，预期被拒绝
	conv, err := srv.Store.GetConversationByPublicID(publicID)
	if err != nil {
		t.Fatal(err)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages",
		map[string]any{"msg_type": "text", "content": map[string]any{"text": "老消息"}}, guestAuth)
	if w.Code != http.StatusOK {
		t.Fatal("send old message failed")
	}
	var old struct {
		ID uint64 `json:"id"`
	}
	decodeInto(t, env.Data, &old)
	srv.Store.DB().Exec("UPDATE im_messages SET created_at = ? WHERE id = ?", time.Now().Add(-3*time.Minute), old.ID)
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/messages/"+itoa(old.ID)+"/recall", nil, guestAuth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expired recall status %d", w.Code)
	}
	_ = conv
}

func TestAgentStatsAndQueuePosition(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	token := guestSession(t, r, "guest-stats-1")
	publicID := createConversation(t, r, token)

	// 排队中的会话在访客读取时上报其位置
	w, env := do(t, r, http.MethodGet, "/api/v1/im/conversations/"+publicID+"/messages",
		nil, map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("messages status %d", w.Code)
	}
	var mres struct {
		Conversation struct {
			Status        string `json:"status"`
			QueuePosition int64  `json:"queue_position"`
		} `json:"conversation"`
	}
	decodeInto(t, env.Data, &mres)
	if mres.Conversation.Status == "queued" && mres.Conversation.QueuePosition != 1 {
		t.Fatalf("queue position = %d", mres.Conversation.QueuePosition)
	}

	// 关闭并评分，为统计聚合提供数据
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/agent/conversations/"+publicID+"/close",
		map[string]any{"reason": "agent"}, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("close status %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/im/conversations/"+publicID+"/rating",
		map[string]any{"score": 4}, map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("rate status %d", w.Code)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/im/agent/stats?days=7", nil, agentHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("stats status %d body %s", w.Code, w.Body.String())
	}
	var sres struct {
		Stats struct {
			ClosedInRange int64          `json:"closed_in_range"`
			RatedInRange  int64          `json:"rated_in_range"`
			AvgRating     *float64       `json:"avg_rating"`
			ScoreDist     map[string]int `json:"score_dist"`
		} `json:"stats"`
	}
	decodeInto(t, env.Data, &sres)
	if sres.Stats.ClosedInRange != 1 || sres.Stats.RatedInRange != 1 {
		t.Fatalf("stats closed=%d rated=%d", sres.Stats.ClosedInRange, sres.Stats.RatedInRange)
	}
	if sres.Stats.AvgRating == nil || *sres.Stats.AvgRating != 4 {
		t.Fatalf("avg rating = %v", sres.Stats.AvgRating)
	}
	if sres.Stats.ScoreDist["4"] != 1 {
		t.Fatalf("score dist = %v", sres.Stats.ScoreDist)
	}
	_ = srv
}

func TestIdleAutoClose(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	token := guestSession(t, r, "guest-idle-1")
	publicID := createConversation(t, r, token)

	conv, err := srv.Store.GetConversationByPublicID(publicID)
	if err != nil {
		t.Fatal(err)
	}
	// 日期回拨：31 分钟没有消息
	srv.Store.DB().Exec("UPDATE im_conversations SET created_at = ?, last_message_at = NULL WHERE id = ?",
		time.Now().Add(-31*time.Minute), conv.ID)

	closed, err := srv.Store.CloseIdleConversations(30*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range closed {
		if c.ID == conv.ID {
			found = true
			if c.Status != "closed" || c.CloseReason != "timeout" {
				t.Fatalf("idle close: status=%s reason=%s", c.Status, c.CloseReason)
			}
		}
	}
	if !found {
		t.Fatal("idle conversation not closed")
	}
}

func itoa(v uint64) string { return strconv.FormatUint(v, 10) }
