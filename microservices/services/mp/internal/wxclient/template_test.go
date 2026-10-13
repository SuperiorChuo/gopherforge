package wxclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSendTemplate 验证模板消息：POST 到 template/send、body 字段（含 data/url）正确、
// msgid 解析正确。
func TestSendTemplate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/template/send":
			gotMethod = r.Method
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","msgid":200228332}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	msg := TemplateMessage{
		ToUser:     "openid-t",
		TemplateID: "TPL-1",
		URL:        "https://example.com/detail",
		Data: map[string]TemplateField{
			"first":    {Value: "订单已发货", Color: "#173177"},
			"keyword1": {Value: "SF123456"},
		},
	}
	msgID, err := c.SendTemplate(context.Background(), testAccount(), msg)
	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	if msgID != 200228332 {
		t.Fatalf("msgID = %d, want 200228332", msgID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/cgi-bin/message/template/send" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["touser"] != "openid-t" {
		t.Fatalf("touser = %v", gotBody["touser"])
	}
	if gotBody["template_id"] != "TPL-1" {
		t.Fatalf("template_id = %v", gotBody["template_id"])
	}
	if gotBody["url"] != "https://example.com/detail" {
		t.Fatalf("url = %v", gotBody["url"])
	}
	data, ok := gotBody["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing/wrong type: %v", gotBody["data"])
	}
	first, ok := data["first"].(map[string]any)
	if !ok || first["value"] != "订单已发货" || first["color"] != "#173177" {
		t.Fatalf("data.first = %v", data["first"])
	}
	// 无 color 的字段应省略 color（omitempty）。
	kw, ok := data["keyword1"].(map[string]any)
	if !ok || kw["value"] != "SF123456" {
		t.Fatalf("data.keyword1 = %v", data["keyword1"])
	}
	if _, hasColor := kw["color"]; hasColor {
		t.Fatalf("keyword1 should omit empty color, got %v", kw)
	}
}

// TestSendTemplateOmitsEmptyURL 验证 URL 为空时 body 不带 url 字段（omitempty）。
func TestSendTemplateOmitsEmptyURL(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/template/send":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","msgid":1}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.SendTemplate(context.Background(), testAccount(), TemplateMessage{
		ToUser:     "o",
		TemplateID: "T",
		Data:       map[string]TemplateField{"first": {Value: "x"}},
	})
	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	if _, has := gotBody["url"]; has {
		t.Fatalf("empty URL should be omitted, got %v", gotBody["url"])
	}
}

// TestSendTemplateWxError 验证 errcode!=0 转 WxError（如 40037 模板不存在）。
func TestSendTemplateWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/message/template/send":
			_, _ = w.Write([]byte(`{"errcode":40037,"errmsg":"invalid template_id"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.SendTemplate(context.Background(), testAccount(), TemplateMessage{ToUser: "o", TemplateID: "bad"})
	we, ok := err.(*WxError)
	if !ok || we.Code != 40037 {
		t.Fatalf("expected WxError 40037, got %T %v", err, err)
	}
}
