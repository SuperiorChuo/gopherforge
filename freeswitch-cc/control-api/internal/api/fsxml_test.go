package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// fakeExtStore 是 extGetter 的内存实现，避免单测依赖 sqlite/真库驱动。
type fakeExtStore map[string]store.Extension

func (f fakeExtStore) GetExtension(ext string) (*store.Extension, error) {
	if e, ok := f[ext]; ok {
		return &e, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func newFSXMLServer(token string, exts fakeExtStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	srv := &Server{Cfg: config.Config{APIToken: token}, ExtLookup: exts}
	return srv.Router()
}

func fsXMLRequest(form url.Values, user, pass string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/fs-xml", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	return req
}

func TestFSXMLDirectoryHit(t *testing.T) {
	exts := fakeExtStore{
		"2001": {Ext: "2001", Password: "s3cret99", Label: "客服 <A>"},
	}
	r := newFSXMLServer("tok-123", exts)

	form := url.Values{}
	form.Set("section", "directory")
	form.Set("tag_name", "domain")
	form.Set("key_value", "192.168.1.1")
	form.Set("user", "2001")
	form.Set("domain", "192.168.1.1")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, fsXMLRequest(form, "fs", "tok-123"))

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/xml") {
		t.Errorf("content-type=%q, want text/xml", ct)
	}
	body := w.Body.String()
	for _, frag := range []string{
		`<section name="directory">`,
		`<domain name="192.168.1.1">`,
		`<user id="2001">`,
		`<param name="password" value="s3cret99"/>`,
		`<variable name="effective_caller_id_number" value="2001"/>`,
		`value="客服 &lt;A&gt;"`, // label XML 转义兜底
	} {
		if !strings.Contains(body, frag) {
			t.Errorf("missing %q in:\n%s", frag, body)
		}
	}
}

// 语音信箱开启的分机：directory 应答里带 vm-password param（密码空则跟随分机号）。
func TestFSXMLDirectoryVoicemail(t *testing.T) {
	exts := fakeExtStore{
		"2002": {Ext: "2002", Password: "pw", VMEnabled: true, VMPassword: "1357"},
		"2003": {Ext: "2003", Password: "pw", VMEnabled: true}, // 密码空 → 跟随分机号
		"2004": {Ext: "2004", Password: "pw"},                  // 未开启 → 无 vm-password
	}
	r := newFSXMLServer("tok-123", exts)

	check := func(user, wantVM string, wantAbsent bool) {
		form := url.Values{}
		form.Set("section", "directory")
		form.Set("user", user)
		form.Set("domain", "192.168.1.1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, fsXMLRequest(form, "fs", "tok-123"))
		body := w.Body.String()
		if wantAbsent {
			if strings.Contains(body, "vm-password") {
				t.Errorf("%s should omit vm-password:\n%s", user, body)
			}
			return
		}
		if !strings.Contains(body, wantVM) {
			t.Errorf("%s missing %q:\n%s", user, wantVM, body)
		}
	}
	check("2002", `<param name="vm-password" value="1357"/>`, false)
	check("2003", `<param name="vm-password" value="2003"/>`, false)
	check("2004", "", true)
}

func TestFSXMLDirectoryMiss(t *testing.T) {
	r := newFSXMLServer("tok-123", fakeExtStore{})

	form := url.Values{}
	form.Set("section", "directory")
	form.Set("user", "9999") // 动态库没有 → 让 FS 回退磁盘文件
	form.Set("domain", "192.168.1.1")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, fsXMLRequest(form, "fs", "tok-123"))

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `<result status="not found"/>`) {
		t.Errorf("want not-found result, got:\n%s", body)
	}
	if strings.Contains(body, "<user ") {
		t.Errorf("miss should not emit a user element:\n%s", body)
	}
}

func TestFSXMLBadAuth(t *testing.T) {
	exts := fakeExtStore{"2001": {Ext: "2001", Password: "s3cret99"}}
	r := newFSXMLServer("tok-123", exts)

	form := url.Values{}
	form.Set("section", "directory")
	form.Set("user", "2001")

	cases := []struct {
		name       string
		user, pass string
	}{
		{"wrong password", "fs", "nope"},
		{"wrong user", "admin", "tok-123"},
		{"no credentials", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, fsXMLRequest(form, tc.user, tc.pass))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("code=%d, want 401 body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// section 非 directory（如 configuration 误投）应答 not found，FS 回退磁盘。
func TestFSXMLNonDirectorySection(t *testing.T) {
	r := newFSXMLServer("tok-123", fakeExtStore{})

	form := url.Values{}
	form.Set("section", "configuration")
	form.Set("key_value", "sofia.conf")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, fsXMLRequest(form, "fs", "tok-123"))

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `<result status="not found"/>`) {
		t.Errorf("want not-found for non-directory section:\n%s", w.Body.String())
	}
}
