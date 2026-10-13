package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeMenuButtons(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "two levels",
			body: `[{"name":"服务","sub_button":[{"type":"click","name":"人工客服","key":"HELP"},{"type":"view","name":"官网","url":"https://example.com"}]},{"type":"miniprogram","name":"小程序","appid":"wx-mini","pagepath":"pages/index","url":"https://example.com/fallback"}]`,
		},
		{name: "empty", body: `[]`, wantErr: "1 到 3"},
		{name: "too many roots", body: `[{"type":"click","name":"a","key":"1"},{"type":"click","name":"b","key":"2"},{"type":"click","name":"c","key":"3"},{"type":"click","name":"d","key":"4"}]`, wantErr: "1 到 3"},
		{name: "third level", body: `[{"name":"一级","sub_button":[{"name":"二级","sub_button":[{"type":"click","name":"三级","key":"x"}]}]}]`, wantErr: "三级菜单"},
		{name: "parent action", body: `[{"type":"click","name":"一级","key":"x","sub_button":[{"type":"click","name":"二级","key":"y"}]}]`, wantErr: "不能再配置"},
		{name: "missing view url", body: `[{"type":"view","name":"官网"}]`, wantErr: "url 必填"},
		{name: "unknown type", body: `[{"type":"unknown","name":"入口","key":"x"}]`, wantErr: "type 不受支持"},
		{name: "leaf fields are mutually exclusive", body: `[{"type":"click","name":"入口","key":"x","url":"https://example.com"}]`, wantErr: "不能包含 url"},
		{name: "unknown field", body: `[{"type":"click","name":"入口","key":"x","unexpected":true}]`, wantErr: "unknown field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := normalizeMenuButtons(json.RawMessage(tt.body))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("normalizeMenuButtons: %v", err)
				}
				if !json.Valid(raw) {
					t.Fatalf("normalized JSON is invalid: %s", raw)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeMenuButtonsSupportsEveryEditorAction(t *testing.T) {
	tests := map[string]string{
		"click":                `[{"type":"click","name":"入口","key":"K"}]`,
		"view":                 `[{"type":"view","name":"入口","url":"https://example.com"}]`,
		"miniprogram":          `[{"type":"miniprogram","name":"入口","appid":"wx-mini","pagepath":"pages/index","url":"https://example.com"}]`,
		"media_id":             `[{"type":"media_id","name":"入口","media_id":"MEDIA"}]`,
		"view_limited":         `[{"type":"view_limited","name":"入口","media_id":"MEDIA"}]`,
		"article_id":           `[{"type":"article_id","name":"入口","article_id":"ARTICLE"}]`,
		"article_view_limited": `[{"type":"article_view_limited","name":"入口","article_id":"ARTICLE"}]`,
		"scancode_push":        `[{"type":"scancode_push","name":"入口","key":"K"}]`,
		"scancode_waitmsg":     `[{"type":"scancode_waitmsg","name":"入口","key":"K"}]`,
		"pic_sysphoto":         `[{"type":"pic_sysphoto","name":"入口","key":"K"}]`,
		"pic_photo_or_album":   `[{"type":"pic_photo_or_album","name":"入口","key":"K"}]`,
		"pic_weixin":           `[{"type":"pic_weixin","name":"入口","key":"K"}]`,
		"location_select":      `[{"type":"location_select","name":"入口","key":"K"}]`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeMenuButtons(json.RawMessage(body)); err != nil {
				t.Fatalf("normalize %s: %v", name, err)
			}
		})
	}
}
