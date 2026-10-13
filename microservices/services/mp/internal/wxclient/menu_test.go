package wxclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateMenu 验证创建菜单：POST 到 menu/create、button 数组透传正确。
func TestCreateMenu(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/menu/create":
			gotMethod = r.Method
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	buttons := json.RawMessage(`[{"type":"click","name":"今日歌曲","key":"V1001_TODAY_MUSIC"}]`)
	if err := c.CreateMenu(context.Background(), testAccount(), buttons); err != nil {
		t.Fatalf("CreateMenu: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/cgi-bin/menu/create" {
		t.Fatalf("path = %q", gotPath)
	}
	// button 应原样透传。
	var wantBtns, gotBtns []map[string]any
	_ = json.Unmarshal(buttons, &wantBtns)
	_ = json.Unmarshal(gotBody["button"], &gotBtns)
	if len(gotBtns) != 1 || gotBtns[0]["key"] != "V1001_TODAY_MUSIC" {
		t.Fatalf("button not passed through: %s", string(gotBody["button"]))
	}
}

// TestGetMenu 验证查询菜单返回原始 JSON。
func TestGetMenu(t *testing.T) {
	var gotMethod string
	rawMenu := `{"menu":{"button":[{"name":"歌曲","sub_button":[]}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/menu/get":
			gotMethod = r.Method
			_, _ = w.Write([]byte(rawMenu))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	got, err := c.GetMenu(context.Background(), testAccount())
	if err != nil {
		t.Fatalf("GetMenu: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	// 语义上应等于 rawMenu（比较解析后的结构，避免空白差异）。
	var wantV, gotV any
	_ = json.Unmarshal([]byte(rawMenu), &wantV)
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("returned menu not valid JSON: %v (%s)", err, string(got))
	}
	wb, _ := json.Marshal(wantV)
	gb, _ := json.Marshal(gotV)
	if string(wb) != string(gb) {
		t.Fatalf("menu = %s, want %s", string(gb), string(wb))
	}
}

// TestGetMenuWxError 验证查询菜单 errcode!=0（如 46003 无菜单数据）转 WxError。
func TestGetMenuWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/menu/get":
			_, _ = w.Write([]byte(`{"errcode":46003,"errmsg":"menu no exist"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetMenu(context.Background(), testAccount())
	we, ok := err.(*WxError)
	if !ok || we.Code != 46003 {
		t.Fatalf("expected WxError 46003, got %T %v", err, err)
	}
}

// TestDeleteMenu 验证删除菜单：GET 到 menu/delete。
func TestDeleteMenu(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/menu/delete":
			gotMethod = r.Method
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.DeleteMenu(context.Background(), testAccount()); err != nil {
		t.Fatalf("DeleteMenu: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/cgi-bin/menu/delete" {
		t.Fatalf("path = %q", gotPath)
	}
}
