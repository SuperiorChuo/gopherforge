package metrics

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeESL 按命令前缀返回预置输出；failAll=true 时所有命令返回错误。
type fakeESL struct {
	responses map[string]string
	failAll   bool
}

func (f *fakeESL) API(cmd string) (string, error) {
	if f.failAll {
		return "", errors.New("esl down")
	}
	// 最长前缀优先：`queue list members X` 不能被 `queue list` 抢走。
	prefixes := make([]string, 0, len(f.responses))
	for p := range f.responses {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if strings.HasPrefix(cmd, prefix) {
			return f.responses[prefix], nil
		}
	}
	return "", errors.New("no stub for " + cmd)
}

func newFakeESL() *fakeESL {
	return &fakeESL{responses: map[string]string{
		"show channels count": "5 total.",
		"show calls count":    "2 total.",
		"callcenter_config queue list members sales@default": "queue|system_epoch|uuid|cid_number|state\n" +
			"sales@default|1000000|u1|1861|Waiting\n+OK\n",
		"callcenter_config queue list": "name|strategy|max_wait_time\nsales@default|ring-all|300\n+OK\n",
		"callcenter_config agent list": "name|type|contact|status|state\n" +
			"1001@default|callback|user/1001|Available|Waiting\n" +
			"1002@default|callback|user/1002|On Break|Waiting\n+OK\n",
		"sofia xmlstatus gateways": "<gateways>" +
			"<gateway><name>gw1</name><state>REGED</state></gateway>" +
			"<gateway><name>gw2</name><state>FAIL_WAIT</state></gateway>" +
			"</gateways>",
	}}
}

func TestCollectorSetsGauges(t *testing.T) {
	m := New()
	c := NewCollector(newFakeESL(), m, 0)
	c.nowUnix = func() int64 { return 1000120 } // sales 等待 120s
	c.collectOnce()

	if v := testutil.ToFloat64(m.ChannelsTotal); v != 5 {
		t.Errorf("channels = %v, want 5", v)
	}
	if v := testutil.ToFloat64(m.CallsTotal); v != 2 {
		t.Errorf("calls = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.QueueMembers.WithLabelValues("sales")); v != 1 {
		t.Errorf("sales members = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.QueueLongestWaitSecs.WithLabelValues("sales")); v != 120 {
		t.Errorf("sales longest wait = %v, want 120", v)
	}
	if v := testutil.ToFloat64(m.Agents.WithLabelValues("Available")); v != 1 {
		t.Errorf("agents Available = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.Agents.WithLabelValues("On Break")); v != 1 {
		t.Errorf("agents On Break = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.GatewayUp.WithLabelValues("gw1")); v != 1 {
		t.Errorf("gw1 up = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.GatewayUp.WithLabelValues("gw2")); v != 0 {
		t.Errorf("gw2 up = %v, want 0", v)
	}
	if v := testutil.ToFloat64(m.ESLUp); v != 1 {
		t.Errorf("esl_up = %v, want 1", v)
	}
}

// ESL 整轮失败时 esl_up=0，且业务 gauge 保留上轮值。
func TestCollectorESLDownPreservesValues(t *testing.T) {
	m := New()
	fake := newFakeESL()
	c := NewCollector(fake, m, 0)
	c.nowUnix = func() int64 { return 1000120 }
	c.collectOnce() // 采一轮好数据

	fake.failAll = true
	c.collectOnce() // 整轮失败

	if v := testutil.ToFloat64(m.ESLUp); v != 0 {
		t.Errorf("esl_up = %v, want 0 after failure", v)
	}
	// 保留上轮值
	if v := testutil.ToFloat64(m.ChannelsTotal); v != 5 {
		t.Errorf("channels after esl down = %v, want preserved 5", v)
	}
	if v := testutil.ToFloat64(m.QueueMembers.WithLabelValues("sales")); v != 1 {
		t.Errorf("sales members after esl down = %v, want preserved 1", v)
	}
}
