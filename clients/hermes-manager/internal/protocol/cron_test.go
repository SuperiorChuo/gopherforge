package protocol

import (
	"testing"
	"time"
)

func TestParseCronExpressionRejects(t *testing.T) {
	bad := []string{
		"", "0 9 * *", "0 9 * * * extra", "60 9 * * *", "0 24 * * *",
		"0 9 0 * *", "0 9 * 13 *", "0 9 * * 8", "x 9 * * *",
		"0 9 * ? *", "? 9 * * *", "0 9 * * * */2", "0 9 * * 1,",
	}
	for _, expr := range bad {
		if _, err := ParseCronExpression(expr); err == nil {
			t.Errorf("ParseCronExpression(%q) should fail", expr)
		}
	}
}

func TestCronMatches(t *testing.T) {
	monday9 := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)  // 周一
	monday915 := time.Date(2026, 8, 3, 9, 15, 0, 0, time.UTC)
	sunday9 := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)   // 周日
	firstOfMonth := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		expr string
		at   time.Time
		want bool
	}{
		{"0 9 * * 1-5", monday9, true},    // 工作日 9 点，周一命中
		{"0 9 * * 1-5", sunday9, false},   // 周日不命中
		{"0 9 * * 1-5", monday915, false}, // 9:15 不命中
		{"*/15 * * * *", monday915, true}, // 每 15 分钟
		{"0 0 1 * *", firstOfMonth, true}, // 每月 1 号 0 点
		{"0 9 * * 1", monday9, true},      // 仅周一
		{"0 9 * * 0", sunday9, true},      // 周日 0 段
		{"0 9 ? * 1", monday9, true},      // ? 日 + 周 1
		{"30 8 * * 1,3,5", monday915, false},
		// 周日的 7 写法：只在周日命中，绝不能在其他日子触发（曾有每天误触发 bug）
		{"0 9 * * 7", sunday9, true},
		{"0 9 * * 7", monday9, false},
		{"0 9 * * 5-7", sunday9, true},                                       // 范围含 7 → 周日命中
		{"0 9 * * 5-7", time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC), false},  // 周三不命中
		{"0 9 * * 5-7", time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC), true},   // 周五命中
		// */n 步进按 vixie 从字段最小值起步：日字段 */2 = 1,3,5…（非 2,4,6…）
		{"0 0 */2 * *", firstOfMonth, true},
		{"0 0 */2 * *", time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), false},
		{"0 0 */2 * *", time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), true},
		// 分钟字段 min=0，*/15 语义不受起步修正影响
		{"*/15 * * * *", monday9, true},
	}
	for _, c := range cases {
		spec, err := ParseCronExpression(c.expr)
		if err != nil {
			t.Fatalf("parse %q: %v", c.expr, err)
		}
		if got := spec.Matches(c.at); got != c.want {
			t.Errorf("Matches(%q @ %v) = %v, want %v", c.expr, c.at, got, c.want)
		}
	}
}
