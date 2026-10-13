package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseParked(t *testing.T) {
	// 典型 valet_info 返回：extension 节点带 id + 主叫属性
	raw := `<valet_lot name="cclot">
  <extension id="6101" uuid="abc-1" caller_id_number="18800001111" caller_id_name="Alice"/>
  <extension id="6102" cid_num="18800002222"/>
  <extension ext="6103"/>
</valet_lot>`
	got := parseParked(raw)
	if len(got) != 3 {
		t.Fatalf("want 3 slots, got %d (%+v)", len(got), got)
	}
	if got[0].Slot != "6101" || got[0].Caller != "18800001111" {
		t.Errorf("slot0 = %+v", got[0])
	}
	if got[1].Slot != "6102" || got[1].Caller != "18800002222" {
		t.Errorf("slot1 = %+v", got[1])
	}
	if got[2].Slot != "6103" || got[2].Caller != "" {
		t.Errorf("slot2 = %+v", got[2])
	}
}

func TestParseParkedEmptyAndError(t *testing.T) {
	for _, raw := range []string{"", "  ", "-ERR no such lot", "not xml at all"} {
		if got := parseParked(raw); len(got) != 0 {
			t.Errorf("raw %q: want empty, got %+v", raw, got)
		}
	}
}

func TestTransferValidatesInput(t *testing.T) {
	r := newControlServer(nil) // 校验不通过不应触达 ESL
	bad := []string{
		`{"call_id":"x; hupall","to":"1001"}`,                     // 非法 uuid
		`{"call_id":"fee34799-dc2c-4b04","to":"1001 XML public"}`, // 注入 to
		`{"call_id":"fee34799-dc2c-4b04"}`,                        // 缺 to
	}
	for _, body := range bad {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/calls/transfer", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: code = %d, want 400", body, w.Code)
		}
	}
}

func TestTransferHappyPath(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "api uuid_transfer fee34799-dc2c-4b04-94aa-4aeec9891d3c 1001 XML default"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/transfer",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","to":"1001"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTransferBoth(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "api uuid_transfer fee34799-dc2c-4b04-94aa-4aeec9891d3c -both 5000 XML default"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/transfer",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","to":"5000","both":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHoldActions(t *testing.T) {
	cases := map[string]string{
		"on":     "api uuid_hold fee34799-dc2c-4b04-94aa-4aeec9891d3c",
		"off":    "api uuid_hold off fee34799-dc2c-4b04-94aa-4aeec9891d3c",
		"toggle": "api uuid_hold toggle fee34799-dc2c-4b04-94aa-4aeec9891d3c",
	}
	for action, wantCmd := range cases {
		eslc := fakeESL(t, func(cmd string) string {
			if cmd != wantCmd {
				t.Errorf("action %s: cmd = %q, want %q", action, cmd, wantCmd)
			}
			return "+OK"
		})
		r := newControlServer(eslc)
		w := httptest.NewRecorder()
		body := `{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","action":"` + action + `"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/calls/hold", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("action %s: code=%d body=%s", action, w.Code, w.Body.String())
		}
	}

	// 非法 action 直接 400
	r := newControlServer(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/hold",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","action":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid action: code=%d", w.Code)
	}
}

func TestParkValidatesSlot(t *testing.T) {
	r := newControlServer(nil)
	for _, body := range []string{
		`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","slot":"6000"}`, // 不在 61xx 段
		`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","slot":"6200"}`,
		`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","slot":"abc"}`,
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/calls/park", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: code = %d, want 400", body, w.Code)
		}
	}
}

func TestParkHappyPath(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "api uuid_transfer fee34799-dc2c-4b04-94aa-4aeec9891d3c 6101 XML default"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/park",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","slot":"6101"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDTMFValidatesInput(t *testing.T) {
	r := newControlServer(nil)
	for _, body := range []string{
		`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","digits":"12a"}`, // 含字母
		`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","digits":""}`,    // 空
		`{"call_id":"x; hupall","digits":"123"}`,                            // 非法 uuid
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/calls/dtmf", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: code = %d, want 400", body, w.Code)
		}
	}
}

func TestDTMFHappyPath(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "api uuid_send_dtmf fee34799-dc2c-4b04-94aa-4aeec9891d3c 1234#"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/dtmf",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c","digits":"1234#"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListParkedEmpty(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		if cmd != "api valet_info cclot" {
			t.Errorf("cmd = %q", cmd)
		}
		return `<valet_lot name="cclot"></valet_lot>`
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/parked", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"list":[]`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}
