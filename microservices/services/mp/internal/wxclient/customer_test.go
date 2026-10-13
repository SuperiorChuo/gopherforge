package wxclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSendCustomerText 验证文本客服消息：POST 到 custom/send、body 结构正确。
func TestSendCustomerText(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/custom/send":
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
	if err := c.SendCustomerText(context.Background(), testAccount(), "openid-1", "你好"); err != nil {
		t.Fatalf("SendCustomerText: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/cgi-bin/message/custom/send" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["touser"] != "openid-1" {
		t.Fatalf("touser = %v, want openid-1", gotBody["touser"])
	}
	if gotBody["msgtype"] != "text" {
		t.Fatalf("msgtype = %v, want text", gotBody["msgtype"])
	}
	text, ok := gotBody["text"].(map[string]any)
	if !ok || text["content"] != "你好" {
		t.Fatalf("text = %v, want content=你好", gotBody["text"])
	}
}

// TestSendCustomerImage 验证图片客服消息 body 里 media_id 正确。
func TestSendCustomerImage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/custom/send":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.SendCustomerImage(context.Background(), testAccount(), "openid-2", "MEDIA-9"); err != nil {
		t.Fatalf("SendCustomerImage: %v", err)
	}
	if gotBody["msgtype"] != "image" {
		t.Fatalf("msgtype = %v, want image", gotBody["msgtype"])
	}
	image, ok := gotBody["image"].(map[string]any)
	if !ok || image["media_id"] != "MEDIA-9" {
		t.Fatalf("image = %v, want media_id=MEDIA-9", gotBody["image"])
	}
	if gotBody["touser"] != "openid-2" {
		t.Fatalf("touser = %v", gotBody["touser"])
	}
}

// TestSendCustomerWxError 验证客服消息 errcode!=0 转 WxError（如 45015 客服窗口超时）。
func TestSendCustomerWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/custom/send":
			_, _ = w.Write([]byte(`{"errcode":45015,"errmsg":"response out of time limit"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.SendCustomerText(context.Background(), testAccount(), "openid-3", "hi")
	we, ok := err.(*WxError)
	if !ok || we.Code != 45015 {
		t.Fatalf("expected WxError 45015, got %T %v", err, err)
	}
}
