package form

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustSchema(t *testing.T, raw string) *Schema {
	t.Helper()
	s, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}

const demoSchema = `{"version":1,"fields":[
  {"key":"amount_cents","label":"金额","type":"amount","required":true,"min":1},
  {"key":"reason","label":"事由","type":"textarea","required":true},
  {"key":"urgent","label":"加急","type":"switch"},
  {"key":"category","label":"类别","type":"select","options":["差旅","办公","其他"]}
]}`

// 空/null/{} 视为无表单；结构校验覆盖 key 合法性、重复、选项。
func TestParseAndValidate(t *testing.T) {
	for _, raw := range []string{"", "null", "{}"} {
		if s, err := Parse([]byte(raw)); err != nil || s != nil {
			t.Fatalf("%q 应视为无表单, got %v %v", raw, s, err)
		}
	}
	s := mustSchema(t, demoSchema)
	if err := s.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got := s.Keys(); len(got) != 4 || got[0] != "amount_cents" {
		t.Fatalf("keys 不符: %v", got)
	}

	bad := []string{
		`{"fields":[{"key":"Bad-Key","label":"x","type":"input"}]}`,
		`{"fields":[{"key":"a","label":"x","type":"input"},{"key":"a","label":"y","type":"input"}]}`,
		`{"fields":[{"key":"a","label":"x","type":"select"}]}`,
		`{"fields":[{"key":"a","label":"","type":"input"}]}`,
		`{"fields":[{"key":"a","label":"x","type":"unknown"}]}`,
	}
	for _, raw := range bad {
		s := mustSchema(t, raw)
		if err := s.Validate(); err == nil {
			t.Fatalf("应校验失败: %s", raw)
		}
	}
}

// 快照权威校验：必填/类型/选项/范围/未声明字段拒绝，规范化只留声明字段。
func TestValidateSnapshot(t *testing.T) {
	s := mustSchema(t, demoSchema)

	okRaw := []byte(`{"amount_cents":30000,"reason":"采购","urgent":true,"category":"办公"}`)
	out, err := s.ValidateSnapshot(okRaw)
	if err != nil {
		t.Fatalf("合法快照: %v", err)
	}
	m := map[string]any{}
	_ = json.Unmarshal(out, &m)
	if m["amount_cents"].(float64) != 30000 || m["urgent"] != true {
		t.Fatalf("规范化结果不符: %v", m)
	}

	cases := []struct {
		raw  string
		want string
	}{
		{`{"reason":"x"}`, "必填"},                                        // 缺金额
		{`{"amount_cents":"30000","reason":"x"}`, "数字"},                 // 金额非数字
		{`{"amount_cents":300.5,"reason":"x"}`, "整数分"},                  // 非整数分
		{`{"amount_cents":0,"reason":"x"}`, "不能小于"},                     // min
		{`{"amount_cents":1,"reason":"x","category":"不存在"}`, "选项"},     // 非法选项
		{`{"amount_cents":1,"reason":"x","hacked":"1"}`, "未声明"},         // 夹带字段
		{`{"amount_cents":1,"reason":"x","urgent":"yes"}`, "开关"},        // 类型错
	}
	for _, tc := range cases {
		if _, err := s.ValidateSnapshot([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s 应报「%s」, got %v", tc.raw, tc.want, err)
		}
	}
}

// 字段权限：仅 hidden、须引用已声明字段。
func TestValidateFieldPerms(t *testing.T) {
	s := mustSchema(t, demoSchema)
	if err := s.ValidateFieldPerms("审批", map[string]string{"amount_cents": "hidden"}); err != nil {
		t.Fatalf("合法权限: %v", err)
	}
	if err := s.ValidateFieldPerms("审批", map[string]string{"amount_cents": "readonly"}); err == nil {
		t.Fatal("非法权限值应拒绝")
	}
	if err := s.ValidateFieldPerms("审批", map[string]string{"ghost": "hidden"}); err == nil {
		t.Fatal("未声明字段应拒绝")
	}
}
