// 条件表达式（M3）：简单比较 + AND/OR 组合，不做脚本。
// 求值输入是实例发起时冻结的 form_snapshot；叶子做类型宽松比较——
// 双方均可解析为数字时按 float64 数值比较（金额恒为 amount_cents 整数分，
// float64 在 2^53 内精确承载），否则 eq/ne/in 退化为字符串比较，
// gt/gte/lt/lte 直接报错。求值失败（字段缺失/类型错乱）由引擎挂起实例，
// 不静默走 default（§3.2，避免错批）。
package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 表达式操作符。
const (
	OpAnd = "and"
	OpOr  = "or"
	OpGt  = "gt"
	OpGte = "gte"
	OpLt  = "lt"
	OpLte = "lte"
	OpEq  = "eq"
	OpNe  = "ne"
	OpIn  = "in"
)

// Expr 条件表达式节点：op ∈ {and,or} 时用 Items 组合，否则为叶子比较。
type Expr struct {
	Op    string          `json:"op"`
	Items []Expr          `json:"items,omitempty"`
	Field string          `json:"field,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// ParseExpr 解析分支 expr JSON（null / 空 视为 default，返回 nil）。
func ParseExpr(raw json.RawMessage) (*Expr, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var e Expr
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("条件表达式 JSON 解析失败: %w", err)
	}
	return &e, nil
}

// ValidateExpr 发布时校验：操作符合法、字段已在发起节点声明、值形态匹配。
func ValidateExpr(e *Expr, formFields []string) error {
	if e == nil {
		return errors.New("条件表达式为空")
	}
	switch e.Op {
	case OpAnd, OpOr:
		if len(e.Items) == 0 {
			return fmt.Errorf("%s 组合至少需要一个子条件", e.Op)
		}
		for i := range e.Items {
			if err := ValidateExpr(&e.Items[i], formFields); err != nil {
				return err
			}
		}
		return nil
	case OpGt, OpGte, OpLt, OpLte, OpEq, OpNe, OpIn:
		if strings.TrimSpace(e.Field) == "" {
			return errors.New("比较条件缺少字段名")
		}
		declared := false
		for _, f := range formFields {
			if f == e.Field {
				declared = true
				break
			}
		}
		if !declared {
			return fmt.Errorf("条件字段「%s」未在发起节点表单字段中声明", e.Field)
		}
		vals, isArray, err := parseValue(e.Value)
		if err != nil {
			return fmt.Errorf("字段「%s」的比较值非法: %w", e.Field, err)
		}
		if e.Op == OpIn {
			if !isArray || len(vals) == 0 {
				return fmt.Errorf("字段「%s」的 in 条件需要非空数组值", e.Field)
			}
		} else if isArray {
			return fmt.Errorf("字段「%s」的 %s 条件不接受数组值", e.Field, e.Op)
		}
		return nil
	default:
		return fmt.Errorf("条件操作符未知: %s", e.Op)
	}
}

// EvalExpr 求值：snapshot 为表单快照反序列化后的 map。
// 任何异常（字段缺失/类型不可比）返回 error，由引擎挂起实例。
func EvalExpr(e *Expr, snapshot map[string]any) (bool, error) {
	if e == nil {
		return false, errors.New("条件表达式为空")
	}
	switch e.Op {
	case OpAnd:
		for i := range e.Items {
			hit, err := EvalExpr(&e.Items[i], snapshot)
			if err != nil {
				return false, err
			}
			if !hit {
				return false, nil
			}
		}
		return len(e.Items) > 0, nil
	case OpOr:
		for i := range e.Items {
			hit, err := EvalExpr(&e.Items[i], snapshot)
			if err != nil {
				return false, err
			}
			if hit {
				return true, nil
			}
		}
		return false, nil
	case OpGt, OpGte, OpLt, OpLte, OpEq, OpNe, OpIn:
		got, exists := snapshot[e.Field]
		if !exists || got == nil {
			return false, fmt.Errorf("表单快照缺少条件字段「%s」", e.Field)
		}
		vals, _, err := parseValue(e.Value)
		if err != nil {
			return false, fmt.Errorf("字段「%s」的比较值非法: %w", e.Field, err)
		}
		if e.Op == OpIn {
			for _, v := range vals {
				hit, err := leafCompare(OpEq, got, v)
				if err != nil {
					return false, err
				}
				if hit {
					return true, nil
				}
			}
			return false, nil
		}
		if len(vals) != 1 {
			return false, fmt.Errorf("字段「%s」的 %s 条件不接受数组值", e.Field, e.Op)
		}
		return leafCompare(e.Op, got, vals[0])
	default:
		return false, fmt.Errorf("条件操作符未知: %s", e.Op)
	}
}

// leafCompare 叶子比较：双方均为数字（或数字字符串）→ 数值比较；
// 否则 eq/ne 字符串比较，大小比较报错。
func leafCompare(op string, got any, want any) (bool, error) {
	gn, gok := toNumber(got)
	wn, wok := toNumber(want)
	if gok && wok {
		switch op {
		case OpGt:
			return gn > wn, nil
		case OpGte:
			return gn >= wn, nil
		case OpLt:
			return gn < wn, nil
		case OpLte:
			return gn <= wn, nil
		case OpEq:
			return gn == wn, nil
		case OpNe:
			return gn != wn, nil
		}
	}
	switch op {
	case OpEq:
		return toString(got) == toString(want), nil
	case OpNe:
		return toString(got) != toString(want), nil
	}
	return false, fmt.Errorf("%s 条件要求数值，实际值为「%v」", op, got)
}

// parseValue 解析叶子比较值：标量返回单元素切片，数组按元素展开。
func parseValue(raw json.RawMessage) (vals []any, isArray bool, err error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, false, errors.New("比较值不能为空")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false, err
	}
	if arr, ok := v.([]any); ok {
		return arr, true, nil
	}
	switch v.(type) {
	case float64, string, bool:
		return []any{v}, false, nil
	default:
		return nil, false, fmt.Errorf("不支持的值类型 %T", v)
	}
}

func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}
