package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const maxMenuPayloadBytes = 64 * 1024

type menuButton struct {
	Type      string       `json:"type,omitempty"`
	Name      string       `json:"name"`
	Key       string       `json:"key,omitempty"`
	URL       string       `json:"url,omitempty"`
	AppID     string       `json:"appid,omitempty"`
	PagePath  string       `json:"pagepath,omitempty"`
	MediaID   string       `json:"media_id,omitempty"`
	ArticleID string       `json:"article_id,omitempty"`
	SubButton []menuButton `json:"sub_button,omitempty"`
}

func normalizeMenuButtons(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maxMenuPayloadBytes {
		return nil, fmt.Errorf("buttons 必填且不能超过 %d KB", maxMenuPayloadBytes/1024)
	}
	var buttons []menuButton
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&buttons); err != nil {
		return nil, fmt.Errorf("buttons 格式错误: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, fmt.Errorf("buttons 格式错误: %w", err)
	}
	if len(buttons) == 0 || len(buttons) > 3 {
		return nil, fmt.Errorf("一级菜单数量必须为 1 到 3 个")
	}
	for i := range buttons {
		if err := validateMenuButton(&buttons[i], 1, fmt.Sprintf("菜单 %d", i+1)); err != nil {
			return nil, err
		}
	}
	normalized, err := json.Marshal(buttons)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("只能包含一个 JSON 值")
		}
		return err
	}
	return nil
}

func validateMenuButton(button *menuButton, level int, path string) error {
	button.Name = strings.TrimSpace(button.Name)
	button.Type = strings.TrimSpace(button.Type)
	button.Key = strings.TrimSpace(button.Key)
	button.URL = strings.TrimSpace(button.URL)
	button.AppID = strings.TrimSpace(button.AppID)
	button.PagePath = strings.TrimSpace(button.PagePath)
	button.MediaID = strings.TrimSpace(button.MediaID)
	button.ArticleID = strings.TrimSpace(button.ArticleID)

	maxNameWeight := 8
	if level == 2 {
		maxNameWeight = 16
	}
	if button.Name == "" || menuNameWeight(button.Name) > maxNameWeight {
		return fmt.Errorf("%s 名称必填，且不能超过 %d 个字母或 %d 个汉字", path, maxNameWeight, maxNameWeight/2)
	}
	if len(button.SubButton) > 0 {
		if level != 1 {
			return fmt.Errorf("%s 不支持三级菜单", path)
		}
		if len(button.SubButton) > 5 {
			return fmt.Errorf("%s 最多包含 5 个二级菜单", path)
		}
		if button.Type != "" || button.Key != "" || button.URL != "" || button.AppID != "" ||
			button.PagePath != "" || button.MediaID != "" || button.ArticleID != "" {
			return fmt.Errorf("%s 包含二级菜单时不能再配置点击动作", path)
		}
		for i := range button.SubButton {
			if err := validateMenuButton(&button.SubButton[i], 2, fmt.Sprintf("%s.%d", path, i+1)); err != nil {
				return err
			}
		}
		return nil
	}

	allowedFields := []string(nil)
	switch button.Type {
	case "click", "scancode_push", "scancode_waitmsg", "pic_sysphoto", "pic_photo_or_album", "pic_weixin", "location_select":
		allowedFields = []string{"key"}
		if button.Key == "" || len([]byte(button.Key)) > 128 {
			return fmt.Errorf("%s 的 key 必填且不能超过 128 字节", path)
		}
	case "view":
		allowedFields = []string{"url"}
		if err := validateMenuURL(button.URL); err != nil {
			return fmt.Errorf("%s 的 url %w", path, err)
		}
	case "miniprogram":
		allowedFields = []string{"appid", "pagepath", "url"}
		if button.AppID == "" || len([]byte(button.AppID)) > 64 {
			return fmt.Errorf("%s 的 appid 必填且不能超过 64 字节", path)
		}
		if button.PagePath == "" || len([]byte(button.PagePath)) > 1024 {
			return fmt.Errorf("%s 的 pagepath 必填且不能超过 1024 字节", path)
		}
		if err := validateMenuURL(button.URL); err != nil {
			return fmt.Errorf("%s 的备用 url %w", path, err)
		}
	case "media_id", "view_limited":
		allowedFields = []string{"media_id"}
		if button.MediaID == "" || len([]byte(button.MediaID)) > 128 {
			return fmt.Errorf("%s 的 media_id 必填且不能超过 128 字节", path)
		}
	case "article_id", "article_view_limited":
		allowedFields = []string{"article_id"}
		if button.ArticleID == "" || len([]byte(button.ArticleID)) > 128 {
			return fmt.Errorf("%s 的 article_id 必填且不能超过 128 字节", path)
		}
	default:
		return fmt.Errorf("%s 的 type 不受支持", path)
	}
	if field := unexpectedMenuActionField(button, allowedFields...); field != "" {
		return fmt.Errorf("%s 的 type %s 不能包含 %s 字段", path, button.Type, field)
	}
	return nil
}

func unexpectedMenuActionField(button *menuButton, allowed ...string) string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "key", value: button.Key},
		{name: "url", value: button.URL},
		{name: "appid", value: button.AppID},
		{name: "pagepath", value: button.PagePath},
		{name: "media_id", value: button.MediaID},
		{name: "article_id", value: button.ArticleID},
	}
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		if _, ok := allowedSet[field.name]; !ok {
			return field.name
		}
	}
	return ""
}

func validateMenuURL(raw string) error {
	if raw == "" || len([]byte(raw)) > 1024 {
		return fmt.Errorf("必填且不能超过 1024 字节")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("必须是完整的 http/https 地址")
	}
	return nil
}

func menuNameWeight(raw string) int {
	weight := 0
	for _, r := range raw {
		if r <= 127 {
			weight++
		} else {
			weight += 2
		}
	}
	return weight
}
