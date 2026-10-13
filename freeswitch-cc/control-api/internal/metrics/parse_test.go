package metrics

import "testing"

func TestParseCountTotal(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{"channels", "12 total.", 12, true},
		{"zero", "0 total.", 0, true},
		{"trailing whitespace", "  7 total.\n", 7, true},
		{"empty", "", 0, false},
		{"non-numeric", "+OK\n", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseCountTotal(tc.raw)
			if got != tc.want || ok != tc.ok {
				t.Errorf("parseCountTotal(%q) = (%d,%v), want (%d,%v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseQueueNames(t *testing.T) {
	raw := "name|strategy|moh_sound|time_base_score|tier_rules_apply|max_wait_time\n" +
		"sales@default|longest-idle-agent|$${moh_uri}|queue|false|300\n" +
		"support@default|ring-all|$${moh_uri}|queue|false|600\n" +
		"+OK\n"
	got := parseQueueNames(raw)
	want := []string{"sales", "support"}
	if len(got) != len(want) {
		t.Fatalf("parseQueueNames len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("queue[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseQueueMembers(t *testing.T) {
	// system_epoch 相对 now 的等待秒数：150、40、（Answered/Abandoned 不计等待）
	now := int64(1_000_200)
	raw := "queue|system_epoch|joined_epoch|uuid|session_uuid|cid_number|cid_name|serving_agent|serving_system|state\n" +
		"sales@default|1000050|1000050|u1|s1|1861|Alice||default|Waiting\n" +
		"sales@default|1000160|1000160|u2|s2|1862|Bob||default|Trying\n" +
		"sales@default|999000|999000|u3|s3|1863|Carol|1001@default|default|Answered\n" +
		"sales@default|998000|998000|u4|s4|1864|Dave||default|Abandoned\n" +
		"+OK\n"
	st := parseQueueMembers(raw, now)
	if st.Members != 2 {
		t.Errorf("Members = %d, want 2", st.Members)
	}
	if st.LongestWaitSecs != 150 { // now - 1000050
		t.Errorf("LongestWaitSecs = %d, want 150", st.LongestWaitSecs)
	}
	if st.Answered != 1 {
		t.Errorf("Answered = %d, want 1", st.Answered)
	}
	if st.Abandoned != 1 {
		t.Errorf("Abandoned = %d, want 1", st.Abandoned)
	}
}

func TestParseQueueMembersEmpty(t *testing.T) {
	// 队列空时 members 输出仅表头或 +OK。
	st := parseQueueMembers("+OK\n", 1000)
	if st.Members != 0 || st.LongestWaitSecs != 0 {
		t.Errorf("empty queue = %+v, want zero", st)
	}
}

func TestParseAgentStatuses(t *testing.T) {
	raw := "name|type|contact|status|state|max_no_answer|wrap_up_time|reject_delay_time|busy_delay_time|no_answer_delay_time|last_bridge_start|last_bridge_end|last_offered_call|last_status_change|no_answer_count|calls_answered|talk_time|ready_time|external_calls_count\n" +
		"1001@default|callback|user/1001|Available|Waiting|0|10|0|0|0|0|0|0|0|0|3|120|0|0\n" +
		"1002@default|callback|user/1002|Available|Waiting|0|10|0|0|0|0|0|0|0|0|1|30|0|0\n" +
		"1003@default|callback|user/1003|On Break|Waiting|0|10|0|0|0|0|0|0|0|0|0|0|0|0\n" +
		"1004@default|callback|user/1004|Logged Out|Waiting|0|10|0|0|0|0|0|0|0|0|0|0|0|0\n" +
		"+OK\n"
	got := parseAgentStatuses(raw)
	if got["Available"] != 2 {
		t.Errorf("Available = %d, want 2", got["Available"])
	}
	if got["On Break"] != 1 {
		t.Errorf("On Break = %d, want 1", got["On Break"])
	}
	if got["Logged Out"] != 1 {
		t.Errorf("Logged Out = %d, want 1", got["Logged Out"])
	}
}

func TestParseGatewayStates(t *testing.T) {
	raw := `<gateways>
  <gateway>
    <name>gw_reg</name>
    <profile>external</profile>
    <state>REGED</state>
  </gateway>
  <gateway>
    <name>gw_out</name>
    <profile>external</profile>
    <state>NOREG</state>
  </gateway>
  <gateway>
    <name>gw_fail</name>
    <profile>external</profile>
    <state>FAIL_WAIT</state>
  </gateway>
</gateways>`
	got := parseGatewayStates(raw)
	if len(got) != 3 {
		t.Fatalf("parseGatewayStates len = %d (%v), want 3", len(got), got)
	}
	want := map[string]string{"gw_reg": "REGED", "gw_out": "NOREG", "gw_fail": "FAIL_WAIT"}
	for _, g := range got {
		if want[g.Name] != g.State {
			t.Errorf("gateway %q state = %q, want %q", g.Name, g.State, want[g.Name])
		}
	}
}

func TestParseGatewayStatesEmpty(t *testing.T) {
	// 无网关时 FS 回空 <gateways/> 或空串。
	if got := parseGatewayStates("<gateways></gateways>"); len(got) != 0 {
		t.Errorf("empty gateways = %v, want nil", got)
	}
	if got := parseGatewayStates(""); got != nil {
		t.Errorf("blank input = %v, want nil", got)
	}
	if got := parseGatewayStates("not xml at all"); got != nil {
		t.Errorf("garbage input = %v, want nil", got)
	}
}

func TestGatewayUp(t *testing.T) {
	cases := map[string]float64{
		"REGED": 1, "NOREG": 1, "reged": 1,
		"FAIL_WAIT": 0, "UNREGED": 0, "TRYING": 0, "": 0, "UNKNOWN": 0,
	}
	for state, want := range cases {
		if got := gatewayUp(state); got != want {
			t.Errorf("gatewayUp(%q) = %v, want %v", state, got, want)
		}
	}
}
