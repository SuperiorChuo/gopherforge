package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 网关头模拟坐席身份（与 read_ratelimit_test 同约定）
func adminReq(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "9")
	req.Header.Set("X-Auth-Username", "agent9")
	req.Header.Set("X-Auth-Permissions", "*")
	return req
}

func TestAdminCreateSite(t *testing.T) {
	r := newTestRouter(t)

	// 创建：生成 app_key/app_secret，snippet 随返回
	w := httptest.NewRecorder()
	r.ServeHTTP(w, adminReq(t, http.MethodPost, "/api/v1/im/admin/sites", map[string]any{
		"name":            "官网站点",
		"welcome_text":    "您好，有什么可以帮您？",
		"allowed_origins": []string{"https://www.example.com"},
		"bot_enabled":     false,
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Site struct {
				ID         uint64 `json:"id"`
				AppKey     string `json:"app_key"`
				Name       string `json:"name"`
				Status     int16  `json:"status"`
				BotEnabled bool   `json:"bot_enabled"`
			} `json:"site"`
			Snippet string `json:"snippet"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	site := resp.Data.Site
	if site.ID == 0 || site.Name != "官网站点" || site.Status != 1 || site.BotEnabled {
		t.Fatalf("unexpected site: %+v", site)
	}
	if len(site.AppKey) != 32 {
		t.Fatalf("app_key should be 32-char uuid hex, got %q", site.AppKey)
	}
	if resp.Data.Snippet == "" {
		t.Fatal("snippet missing")
	}

	// 名称必填
	w = httptest.NewRecorder()
	r.ServeHTTP(w, adminReq(t, http.MethodPost, "/api/v1/im/admin/sites", map[string]any{"name": "  "}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("blank name status = %d", w.Code)
	}

	// 跨租户技能组拒绝
	w = httptest.NewRecorder()
	r.ServeHTTP(w, adminReq(t, http.MethodPost, "/api/v1/im/admin/sites", map[string]any{
		"name":                   "bad-sg",
		"default_skill_group_id": 99999,
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad skill group status = %d", w.Code)
	}

	// 列表应包含 seed 的 demo 站 + 新建站
	w = httptest.NewRecorder()
	r.ServeHTTP(w, adminReq(t, http.MethodGet, "/api/v1/im/admin/sites", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	var listResp struct {
		Data struct {
			List []struct {
				AppKey string `json:"app_key"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Data.List) != 2 {
		t.Fatalf("expect 2 sites, got %d", len(listResp.Data.List))
	}
}
