// Package form 声明式表单 Schema（流程表单模式，设计文档 bpm-form-builder.md）：
// definition.form_schema 的解析、发布校验与发起快照的服务端权威校验。
package form

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// 字段类型。
const (
	TypeInput    = "input"
	TypeTextarea = "textarea"
	TypeNumber   = "number"
	TypeAmount   = "amount" // 存分（int64），渲染时元展示
	TypeSelect   = "select"
	TypeRadio    = "radio"
	TypeDate     = "date" // YYYY-MM-DD 字符串
	TypeSwitch   = "switch"
)

// MaxFields 单表单字段数上限。
const MaxFields = 50

// PermHidden 字段权限（M1 仅"隐藏"）：审批节点 fieldPerms 的合法值。
const PermHidden = "hidden"

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Schema 表单结构。
type Schema struct {
	Version int     `json:"version"`
	Fields  []Field `json:"fields"`
}

// Field 单个字段声明。
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Rows        int      `json:"rows,omitempty"`
}

// Parse 解析 form_schema（空 / "null" / "{}" 视为无表单，返回 nil）。
func Parse(raw []byte) (*Schema, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return nil, nil
	}
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("表单 Schema JSON 解析失败: %w", err)
	}
	if len(s.Fields) == 0 {
		return nil, nil
	}
	return &s, nil
}

// Keys 字段 key 列表（发布时覆盖 start.formFields）。
func (s *Schema) Keys() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.Fields))
	for _, f := range s.Fields {
		out = append(out, f.Key)
	}
	return out
}

// FieldByKey 按 key 取字段；未找到返回 nil。
func (s *Schema) FieldByKey(key string) *Field {
	if s == nil {
		return nil
	}
	for i := range s.Fields {
		if s.Fields[i].Key == key {
			return &s.Fields[i]
		}
	}
	return nil
}

// Validate 发布时校验：key 合法且唯一、类型合法、select/radio 有选项、字段数上限。
func (s *Schema) Validate() error {
	if s == nil {
		return errors.New("表单 Schema 为空")
	}
	if len(s.Fields) > MaxFields {
		return fmt.Errorf("表单字段数超过上限 %d", MaxFields)
	}
	seen := map[string]bool{}
	for i := range s.Fields {
		f := &s.Fields[i]
		if !keyRe.MatchString(f.Key) {
			return fmt.Errorf("字段 key「%s」非法（小写字母开头，仅小写字母/数字/下划线）", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("字段 key 重复: %s", f.Key)
		}
		seen[f.Key] = true
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("字段「%s」缺少显示名", f.Key)
		}
		switch f.Type {
		case TypeInput, TypeTextarea, TypeNumber, TypeAmount, TypeDate, TypeSwitch:
		case TypeSelect, TypeRadio:
			if len(f.Options) == 0 {
				return fmt.Errorf("字段「%s」（%s）需要至少一个选项", f.Label, f.Type)
			}
			optSeen := map[string]bool{}
			for _, opt := range f.Options {
				if strings.TrimSpace(opt) == "" {
					return fmt.Errorf("字段「%s」存在空选项", f.Label)
				}
				if optSeen[opt] {
					return fmt.Errorf("字段「%s」选项重复: %s", f.Label, opt)
				}
				optSeen[opt] = true
			}
		default:
			return fmt.Errorf("字段「%s」类型未知: %s", f.Label, f.Type)
		}
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return fmt.Errorf("字段「%s」min 不能大于 max", f.Label)
		}
	}
	return nil
}

// ValidateFieldPerms 校验审批节点的字段权限：key 须在 Schema 内、值仅 hidden。
func (s *Schema) ValidateFieldPerms(nodeName string, perms map[string]string) error {
	for key, perm := range perms {
		if perm != PermHidden {
			return fmt.Errorf("节点「%s」字段「%s」的权限值未知: %s", nodeName, key, perm)
		}
		if s == nil || s.FieldByKey(key) == nil {
			return fmt.Errorf("节点「%s」的字段权限引用了表单中不存在的字段: %s", nodeName, key)
		}
	}
	return nil
}

// ValidateSnapshot 发起时的服务端权威校验：必填、类型、选项、范围；
// 拒绝 Schema 外字段（防夹带）。返回规范化后的快照（仅含声明字段）。
func (s *Schema) ValidateSnapshot(raw []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("该流程未配置表单")
	}
	m := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("表单数据解析失败: %w", err)
		}
	}
	for key := range m {
		if s.FieldByKey(key) == nil {
			return nil, fmt.Errorf("表单数据包含未声明字段: %s", key)
		}
	}
	out := map[string]any{}
	for i := range s.Fields {
		f := &s.Fields[i]
		v, exists := m[f.Key]
		if !exists || v == nil || v == "" {
			if f.Required {
				return nil, fmt.Errorf("「%s」为必填项", f.Label)
			}
			continue
		}
		switch f.Type {
		case TypeInput, TypeTextarea, TypeDate:
			str, isStr := v.(string)
			if !isStr {
				return nil, fmt.Errorf("「%s」应为文本", f.Label)
			}
			if f.Type == TypeDate && !dateRe.MatchString(str) {
				return nil, fmt.Errorf("「%s」应为 YYYY-MM-DD 日期", f.Label)
			}
			if len(str) > 2000 {
				return nil, fmt.Errorf("「%s」超长", f.Label)
			}
			out[f.Key] = str
		case TypeNumber, TypeAmount:
			n, isNum := v.(float64)
			if !isNum {
				return nil, fmt.Errorf("「%s」应为数字", f.Label)
			}
			if f.Type == TypeAmount && n != float64(int64(n)) {
				return nil, fmt.Errorf("「%s」金额应为整数分", f.Label)
			}
			if f.Min != nil && n < *f.Min {
				return nil, fmt.Errorf("「%s」不能小于 %v", f.Label, *f.Min)
			}
			if f.Max != nil && n > *f.Max {
				return nil, fmt.Errorf("「%s」不能大于 %v", f.Label, *f.Max)
			}
			out[f.Key] = n
		case TypeSelect, TypeRadio:
			str, isStr := v.(string)
			if !isStr {
				return nil, fmt.Errorf("「%s」应为选项值", f.Label)
			}
			hit := false
			for _, opt := range f.Options {
				if opt == str {
					hit = true
					break
				}
			}
			if !hit {
				return nil, fmt.Errorf("「%s」的值不在选项内", f.Label)
			}
			out[f.Key] = str
		case TypeSwitch:
			b, isBool := v.(bool)
			if !isBool {
				return nil, fmt.Errorf("「%s」应为开关值", f.Label)
			}
			out[f.Key] = b
		}
	}
	return json.Marshal(out)
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
