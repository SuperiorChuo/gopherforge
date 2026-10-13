package content

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// MediaItemList 与 StringList 是 JSON 序列化到 text 列的切片类型，便于跨渠道存
// 图片/视频与标签，不引额外表。sqlite/pg 都存成 JSON 字符串。

type MediaItemList []MediaItem

func (l MediaItemList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(l)
	return string(b), err
}

func (l *MediaItemList) Scan(v any) error {
	if v == nil {
		*l = nil
		return nil
	}
	b, err := toBytes(v)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		*l = nil
		return nil
	}
	return json.Unmarshal(b, l)
}

type StringList []string

func (l StringList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(l)
	return string(b), err
}

func (l *StringList) Scan(v any) error {
	if v == nil {
		*l = nil
		return nil
	}
	b, err := toBytes(v)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		*l = nil
		return nil
	}
	return json.Unmarshal(b, l)
}

func toBytes(v any) ([]byte, error) {
	switch t := v.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	default:
		return nil, fmt.Errorf("content: cannot scan %T into JSON list", v)
	}
}
