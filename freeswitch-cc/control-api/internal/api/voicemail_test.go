package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/gin-gonic/gin"
)

// buildVMTree 造出 default/<domain>/<ext>/msg_*.wav 的留言目录，返回 VMDir。
func buildVMTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("default/192.168.1.1/2001/msg_aaa.wav", "RIFFa")
	mk("default/192.168.1.1/2001/msg_bbb.wav", "RIFFb")
	mk("default/192.168.1.1/2002/msg_ccc.wav", "RIFFc")
	// 非 msg 前缀的文件（问候语等）不应被列出
	mk("default/192.168.1.1/2001/greeting_1.wav", "greet")
	return root
}

func newVMServer(t *testing.T, dir string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	srv := &Server{Cfg: config.Config{VMDir: dir}}
	r := gin.New()
	r.GET("/v1/voicemails", srv.listVoicemails)
	r.GET("/v1/voicemails/:ext/:file", srv.getVoicemail)
	r.DELETE("/v1/voicemails/:ext/:file", srv.deleteVoicemail)
	return r
}

func TestVoicemailList(t *testing.T) {
	r := newVMServer(t, buildVMTree(t))

	// 全量：3 条 msg_*.wav（greeting 被排除）
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/voicemails", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"total":3`) {
		t.Errorf("want total 3, got:\n%s", body)
	}
	if strings.Contains(body, "greeting_1") {
		t.Errorf("greeting file must not be listed:\n%s", body)
	}
	if !strings.Contains(body, `"ext":"2001"`) || !strings.Contains(body, `"ext":"2002"`) {
		t.Errorf("ext not derived from path:\n%s", body)
	}

	// 按分机筛选
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/voicemails?ext=2002", nil))
	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("ext filter want 1, got:\n%s", w.Body.String())
	}
}

func TestVoicemailStreamAndDelete(t *testing.T) {
	dir := buildVMTree(t)
	r := newVMServer(t, dir)

	// 播放
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/voicemails/2001/msg_aaa.wav", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "audio/wav" || !strings.Contains(w.Body.String(), "RIFF") {
		t.Fatalf("stream: code=%d ct=%s body=%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}

	// 路径穿越 / 非法名被挡
	for _, target := range []string{
		"/v1/voicemails/2001/..%2Fsecret.wav",
		"/v1/voicemails/2001/msg_aaa.mp3",
		"/v1/voicemails/xx/msg_aaa.wav",
	} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("%s: code=%d", target, w.Code)
		}
	}

	// 删除后文件消失
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v1/voicemails/2001/msg_aaa.wav", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "default/192.168.1.1/2001/msg_aaa.wav")); !os.IsNotExist(err) {
		t.Errorf("file should be removed")
	}

	// 再删已不存在 → 404
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v1/voicemails/2001/msg_aaa.wav", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("second delete: code=%d", w.Code)
	}
}
