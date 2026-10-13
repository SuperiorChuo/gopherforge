package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

func TestCidLookupReturnsBareText(t *testing.T) {
	srv, r := newTestServer(t, "")
	// 建联系人
	body, _ := json.Marshal(map[string]string{"number": "13800000000", "name": "张三客户"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/contacts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert contact: status = %d body=%s", w.Code, w.Body.String())
	}

	// 命中：200 + 纯文本（非 JSON）
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cc/cid?number=13800000000", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cid hit: status = %d", w.Code)
	}
	if w.Body.String() != "张三客户" {
		t.Fatalf("cid body = %q, want 张三客户 (bare text, no envelope)", w.Body.String())
	}

	// 未命中：404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cc/cid?number=19900000000", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cid miss: status = %d, want 404", w.Code)
	}
	_ = srv
}

func TestReportsOverviewAggregates(t *testing.T) {
	srv, r := newTestServer(t, "")
	base := time.Date(2026, 7, 17, 10, 0, 0, 0, time.Local)
	answered := base.Add(15 * time.Second)
	ended := base.Add(75 * time.Second)
	// 呼入接通（等待 15s，通话 60s，坐席 1001，队列 sales）
	c1 := &store.Call{
		TenantID: 1, CallID: "r1", Direction: "inbound", AgentExt: "1001", Queue: "sales",
		Status: "hangup", DurationSec: 60, StartedAt: &base, AnsweredAt: &answered, EndedAt: &ended,
	}
	// 呼入未接通（放弃，队列 sales）
	c2 := &store.Call{
		TenantID: 1, CallID: "r2", Direction: "inbound", Queue: "sales",
		Status: "hangup", StartedAt: &base,
	}
	// 呼出接通
	c3 := &store.Call{
		TenantID: 1, CallID: "r3", Direction: "outbound", AgentExt: "1001",
		Status: "hangup", DurationSec: 30, StartedAt: &base, AnsweredAt: &answered,
	}
	for _, c := range []*store.Call{c1, c2, c3} {
		if err := srv.Store.DB().Create(c).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cc/reports/overview?from=2026-07-17&to=2026-07-17", nil)
	req.Header.Set("X-Auth-User-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reports: status = %d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Data store.ReportOverview `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	d := env.Data
	if d.TotalInbound != 2 || d.TotalOutbound != 1 || d.Total != 3 {
		t.Fatalf("totals wrong: %+v", d)
	}
	if d.Answered != 2 {
		t.Fatalf("answered = %d, want 2", d.Answered)
	}
	if d.Abandoned != 1 {
		t.Fatalf("abandoned = %d, want 1 (inbound not answered)", d.Abandoned)
	}
	// 均通话：(60+30)/2 = 45
	if d.AvgTalkSec != 45 {
		t.Fatalf("avg_talk = %d, want 45", d.AvgTalkSec)
	}
	// 均等待：接通且有 started/answered 的 c1(15) 与 c3(15) → 15
	if d.AvgWaitSec != 15 {
		t.Fatalf("avg_wait = %d, want 15", d.AvgWaitSec)
	}
	if len(d.HourBuckets) != 24 {
		t.Fatalf("hour buckets = %d, want 24", len(d.HourBuckets))
	}
	if d.HourBuckets[10].Inbound != 2 || d.HourBuckets[10].Answered != 2 {
		t.Fatalf("hour 10 bucket wrong: %+v", d.HourBuckets[10])
	}
	if len(d.AgentLeaderboard) != 1 || d.AgentLeaderboard[0].AgentExt != "1001" || d.AgentLeaderboard[0].Answered != 2 {
		t.Fatalf("agent leaderboard wrong: %+v", d.AgentLeaderboard)
	}
	if len(d.QueueLeaderboard) != 1 || d.QueueLeaderboard[0].Entered != 2 || d.QueueLeaderboard[0].Answered != 1 || d.QueueLeaderboard[0].Abandoned != 1 {
		t.Fatalf("queue leaderboard wrong: %+v", d.QueueLeaderboard)
	}
}
