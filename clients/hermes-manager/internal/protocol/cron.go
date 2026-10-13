package protocol

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron 5 段表达式匹配器（M5）。刻意 stdlib-only：manager 模块保持零依赖。
// 语义约定：分 时 日 月 周；? 仅用于日/周；周 0 与 7 均为周日；日与周同时
// 非通配时按 vixie OR（任一命中即算当天命中）。
// 服务端校验器（service validateHermesCronExpression）只做格式/范围校验，匹配在
// 这里执行——两边对表达式的理解必须一致。

type fieldSet struct {
	wildcard bool
	values   map[int]bool
}

// CronExpression 是一次解析好的 5 段表达式。
type CronExpression struct {
	minute, hour, dom, month, dow fieldSet
	// domDowOR 表示日与周都非通配：按 vixie OR 判定（否则非通配的一方胜出）。
	domDowOR bool
}

func parseCronField(field string, min, max int, allowQ bool) (fieldSet, error) {
	field = strings.TrimSpace(field)
	var set fieldSet
	if field == "*" {
		set.wildcard = true
		return set, nil
	}
	if field == "?" {
		if allowQ {
			set.wildcard = true
			return set, nil
		}
		return set, fmt.Errorf("'?' only allowed in day-of-month/day-of-week")
	}
	set.values = map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		step := 1
		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			n, err := strconv.Atoi(strings.TrimSpace(part[slash+1:]))
			if err != nil || n <= 0 {
				return set, fmt.Errorf("invalid cron step %q", part)
			}
			step = n
			part = strings.TrimSpace(part[:slash])
		}
		if part == "*" {
			// vixie 语义：*/n 等价 min-max/n，从字段最小值起步（日字段 */2 = 1,3,5…）
			for v := min; v <= max; v++ {
				if (v-min)%step == 0 {
					set.values[v] = true
				}
			}
			continue
		}
		var lo, hi int
		if dash := strings.IndexByte(part, '-'); dash >= 0 {
			var err error
			lo, err = strconv.Atoi(strings.TrimSpace(part[:dash]))
			if err != nil {
				return set, fmt.Errorf("invalid cron range %q", part)
			}
			hi, err = strconv.Atoi(strings.TrimSpace(part[dash+1:]))
			if err != nil {
				return set, fmt.Errorf("invalid cron range %q", part)
			}
		} else {
			v, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return set, fmt.Errorf("invalid cron value %q", part)
			}
			lo, hi = v, v
		}
		if lo < min || hi > max || lo > hi {
			return set, fmt.Errorf("cron value %q out of range [%d,%d]", part, min, max)
		}
		for v := lo; v <= hi; v++ {
			if (v-lo)%step == 0 {
				set.values[v] = true
			}
		}
	}
	return set, nil
}

// ParseCronExpression 解析并校验 5 段 cron 表达式。
func ParseCronExpression(expr string) (*CronExpression, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields")
	}
	minute, err := parseCronField(fields[0], 0, 59, false)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23, false)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31, true)
	if err != nil {
		return nil, fmt.Errorf("day-of-month: %w", err)
	}
	month, err := parseCronField(fields[3], 1, 12, false)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 7, true)
	if err != nil {
		return nil, fmt.Errorf("day-of-week: %w", err)
	}
	// 周 0 与 7 均为周日：解析期归一化 7→0，Matches 只用 time.Weekday()（周日=0）
	// 对表。不能在匹配期查 set 含不含 7——那与"今天几号"无关，会导致每天误触发。
	if dow.values[7] {
		dow.values[0] = true
		delete(dow.values, 7)
	}
	return &CronExpression{
		minute: minute, hour: hour, dom: dom, month: month, dow: dow,
		domDowOR: !dom.wildcard && !dow.wildcard,
	}, nil
}

// Matches 报告 t 是否命中该表达式（精确到分钟）。
func (c *CronExpression) Matches(t time.Time) bool {
	if c == nil {
		return false
	}
	if !c.minute.matches(t.Minute()) || !c.hour.matches(t.Hour()) || !c.month.matches(int(t.Month())) {
		return false
	}
	day := t.Day()
	// 表达式里的 7 已在解析期归一化成 0，这里直接对 time.Weekday()（周日=0）。
	dowMatches := c.dow.matches(int(t.Weekday()))
	if c.domDowOR {
		// vixie OR：日命中或周命中即算当天命中
		return c.dom.matches(day) || dowMatches
	}
	if !c.dom.wildcard {
		return c.dom.matches(day)
	}
	return dowMatches
}

func (f fieldSet) matches(v int) bool {
	if f.wildcard {
		return true
	}
	return f.values[v]
}
