package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/esl"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
)

// fakeESL answers the auth handshake then replies to each api/bgapi command
// via the respond callback.
func fakeESL(t *testing.T, respond func(cmd string) string) *esl.Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				fmt.Fprint(c, "Content-Type: auth/request\n\n")
				readCmd(br) // auth ...
				fmt.Fprint(c, "Content-Type: command/reply\nReply-Text: +OK accepted\n\n")
				cmd := readCmd(br)
				body := respond(cmd)
				if strings.HasPrefix(cmd, "bgapi ") {
					fmt.Fprintf(c, "Content-Type: command/reply\nReply-Text: %s\n\n", body)
					return
				}
				fmt.Fprintf(c, "Content-Type: api/response\nContent-Length: %d\n\n%s", len(body), body)
			}(conn)
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	return esl.New(host, port, "test")
}

func readCmd(br *bufio.Reader) string {
	var first string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return first
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return first
		}
		if first == "" {
			first = line
		}
	}
}

func newControlServer(eslClient *esl.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	srv := &Server{Cfg: config.Config{}, ESL: eslClient} // empty token = auth off
	return srv.Router()
}

type fakeGWs struct{ list []store.Gateway }

func (f fakeGWs) ListGateways() ([]store.Gateway, error) { return f.list, nil }
func (f fakeGWs) GetGateway(name string) (*store.Gateway, error) {
	for i := range f.list {
		if f.list[i].Name == name {
			return &f.list[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func newAnnounceServer(eslClient *esl.Client, gws []store.Gateway) *gin.Engine {
	gin.SetMode(gin.TestMode)
	srv := &Server{Cfg: config.Config{}, ESL: eslClient, GwSource: fakeGWs{list: gws}}
	return srv.Router()
}

func TestListRegistrations(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		if cmd != "api show registrations as json" {
			t.Errorf("cmd = %q", cmd)
		}
		return `{"row_count":1,"rows":[{"reg_user":"1000","network_ip":"192.168.1.5"}]}`
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/registrations", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reg_user":"1000"`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListChannelsEmpty(t *testing.T) {
	eslc := fakeESL(t, func(string) string { return `{"row_count":0}` })
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/channels", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"list":[]`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestOriginateValidatesInput(t *testing.T) {
	r := newControlServer(nil) // 校验不通过时不应触达 ESL
	for _, body := range []string{
		`{"from_ext":"1000; rm -rf /","to":"1001"}`,
		`{"from_ext":"1000","to":"1001 XML public"}`,
		`{"from_ext":"1000"}`,
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/calls/originate", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: code = %d, want 400", body, w.Code)
		}
	}
}

func TestOriginateHappyPath(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "bgapi originate {origination_caller_id_number=9196,origination_caller_id_name=cc-console,originate_timeout=30,cc_risk_checked=1}user/1000 9196 XML default"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK Job-UUID: abc-123"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/originate", strings.NewReader(`{"from_ext":"1000","to":"9196"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Job-UUID") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestOriginateNamedGatewayMarksOutboundLegForEarlyMedia(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "bgapi originate {origination_caller_id_number=13800138000,origination_caller_id_name=cc-crm,originate_timeout=30,cc_risk_checked=1}user/1000 &bridge([cc_early_media=1]sofia/gateway/gw1/13800138000)"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK Job-UUID: abc-gw"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/originate",
		strings.NewReader(`{"from_ext":"1000","to":"13800138000","gateway":"gw1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHangup(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		if cmd != "api uuid_kill fee34799-dc2c-4b04-94aa-4aeec9891d3c MANAGER_REQUEST" {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/hangup",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	// 非法 uuid 直接 400
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/calls/hangup", strings.NewReader(`{"call_id":"x; hupall"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid: code=%d", w.Code)
	}
}

func TestOriginateInjectsTenantVar(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "bgapi originate {origination_caller_id_number=9196,origination_caller_id_name=cc-console,originate_timeout=30,cc_tenant_id=2,cc_risk_checked=1}user/1000 9196 XML default"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK Job-UUID: abc-456"
	})
	r := newControlServer(eslc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/originate",
		strings.NewReader(`{"from_ext":"1000","to":"9196","tenant_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnnounceHappyPathRewritesDest(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		want := "bgapi originate {ignore_early_media=true,origination_caller_id_number=0211000,origination_caller_id_name=cc-notify,originate_timeout=45,cc_announce=1,cc_early_media=1,cc_tenant_id=2}sofia/gateway/gw1/86138001 &playback(/etc/freeswitch/prompts/vn1.wav)"
		if cmd != want {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK Job-UUID: ann-1"
	})
	gws := []store.Gateway{{
		Name: "gw1", DialPrefix: "00", CallerNumber: "0211000",
	}}
	r := newAnnounceServer(eslc, gws)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/announce",
		strings.NewReader(`{"to":"0086138001","file":"vn1.wav","tenant_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Job-UUID") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnnounceRejectsBadFileAndNoRoute(t *testing.T) {
	r := newAnnounceServer(nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/announce",
		strings.NewReader(`{"to":"13800138000","file":"../etc/passwd"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad file: code=%d", w.Code)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/calls/announce",
		strings.NewReader(`{"to":"13800138000","file":"ok.wav"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "no outbound route") {
		t.Fatalf("no route: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnnounceInjectsVoiceID(t *testing.T) {
	eslc := fakeESL(t, func(cmd string) string {
		if !strings.Contains(cmd, ",cc_voice_id=9") || !strings.Contains(cmd, "cc_announce=1") {
			t.Errorf("cmd = %q", cmd)
		}
		return "+OK Job-UUID: ann-v"
	})
	r := newAnnounceServer(eslc, []store.Gateway{{
		Name: "gw1", DialPrefix: "1", CallerNumber: "0211000",
	}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/announce",
		strings.NewReader(`{"to":"13800138000","file":"vn9.wav","voice_id":9}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnnounceSkipsDisabledGateway(t *testing.T) {
	r := newAnnounceServer(nil, []store.Gateway{
		{Name: "down", DialPrefix: "1", Disabled: true},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/calls/announce",
		strings.NewReader(`{"to":"13800138000","file":"ok.wav"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("disabled: code=%d body=%s", w.Code, w.Body.String())
	}
}
