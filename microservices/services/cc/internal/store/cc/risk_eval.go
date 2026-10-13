package store

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OutboundRiskCounts 滚动窗口内已放行的外呼尝试次数。
type OutboundRiskCounts struct {
	NumberLastHour int
	AgentLastHour  int
	TenantLastMin  int
}

// OutboundRiskInput 一次外呼的风控入参（纯函数，不碰库）。
type OutboundRiskInput struct {
	Callee     string
	FromExt    string
	RatePerMin int64
	Policy     CCRiskPolicy
	Lists      []CCRiskListEntry
	Counts     OutboundRiskCounts
}

// RiskDecision 外呼风控结论。Status 供 HTTP 层原样使用（403 策略拒 / 429 频次）。
type RiskDecision struct {
	Allow  bool
	Code   string
	Reason string
	Status int
}

var (
	shanghaiOnce sync.Once
	shanghaiLoc  *time.Location
)

func AsiaShanghai() *time.Location {
	shanghaiOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*3600)
		}
		shanghaiLoc = loc
	})
	return shanghaiLoc
}

// EvaluateOutbound 按白名单 → 黑名单 → 时段 → 字冠 → 费率上限 → 频次 顺序判定。
// 默认空策略 + 空名单必须放行（不能把未配置当成全拒绝）。
func EvaluateOutbound(now time.Time, loc *time.Location, in OutboundRiskInput) RiskDecision {
	if loc == nil {
		loc = AsiaShanghai()
	}
	now = now.In(loc)
	callee := strings.TrimSpace(in.Callee)

	if matchRiskList(callee, in.Lists, "white") {
		return RiskDecision{Allow: true}
	}
	if matchRiskList(callee, in.Lists, "black") {
		return RiskDecision{Allow: false, Code: "blacklist", Status: 403, Reason: "被叫号码在外呼黑名单中"}
	}

	p := in.Policy
	if denied, reason := denyTimeWindow(now, p.OutboundStart, p.OutboundEnd, p.OutboundWeekdays); denied {
		return RiskDecision{Allow: false, Code: "time_window", Status: 403, Reason: reason}
	}
	if prefix := matchBlockedPrefix(callee, p.BlockedPrefixes); prefix != "" {
		return RiskDecision{Allow: false, Code: "blocked_prefix", Status: 403,
			Reason: "被叫前缀 " + prefix + " 已禁止外呼"}
	}
	if p.MaxRatePerMin > 0 && in.RatePerMin > p.MaxRatePerMin {
		return RiskDecision{Allow: false, Code: "rate_cap", Status: 403, Reason: "该方向费率超过外呼上限"}
	}
	if p.MaxCallsPerNumberPerHour > 0 && in.Counts.NumberLastHour >= p.MaxCallsPerNumberPerHour {
		return RiskDecision{Allow: false, Code: "frequency_number", Status: 429,
			Reason: fmt.Sprintf("同一被叫每小时最多外呼 %d 次", p.MaxCallsPerNumberPerHour)}
	}
	if p.MaxCallsPerAgentPerHour > 0 && in.Counts.AgentLastHour >= p.MaxCallsPerAgentPerHour {
		return RiskDecision{Allow: false, Code: "frequency_agent", Status: 429,
			Reason: fmt.Sprintf("本分机每小时最多外呼 %d 次", p.MaxCallsPerAgentPerHour)}
	}
	if p.MaxCallsPerTenantPerMin > 0 && in.Counts.TenantLastMin >= p.MaxCallsPerTenantPerMin {
		return RiskDecision{Allow: false, Code: "frequency_tenant", Status: 429,
			Reason: fmt.Sprintf("本租户每分钟最多外呼 %d 次", p.MaxCallsPerTenantPerMin)}
	}
	return RiskDecision{Allow: true}
}

func matchRiskList(callee string, lists []CCRiskListEntry, listType string) bool {
	if callee == "" {
		return false
	}
	for _, e := range lists {
		if e.ListType != listType {
			continue
		}
		num := strings.TrimSpace(e.Number)
		if num == "" {
			continue
		}
		switch e.Match {
		case "prefix":
			if strings.HasPrefix(callee, num) {
				return true
			}
		default: // exact 及未知值都按全号
			if callee == num {
				return true
			}
		}
	}
	return false
}

func matchBlockedPrefix(callee, raw string) string {
	if callee == "" || strings.TrimSpace(raw) == "" {
		return ""
	}
	best := ""
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(callee, p) && len(p) > len(best) {
			best = p
		}
	}
	return best
}

func denyTimeWindow(now time.Time, start, end, weekdays string) (bool, string) {
	start, end, weekdays = strings.TrimSpace(start), strings.TrimSpace(end), strings.TrimSpace(weekdays)
	if weekdays != "" {
		allowed, ok := parseWeekdays(weekdays)
		if ok && !allowed[isoWeekday(now)] {
			return true, "当前不在允许外呼的星期"
		}
	}
	if start == "" || end == "" {
		return false, ""
	}
	sh, sm, sok := parseHM(start)
	eh, em, eok := parseHM(end)
	if !sok || !eok {
		return false, ""
	}
	cur := now.Hour()*60 + now.Minute()
	from := sh*60 + sm
	to := eh*60 + em
	inWindow := false
	if from == to {
		inWindow = true // 00:00-00:00 视为全天，避免误杀
	} else if from < to {
		inWindow = cur >= from && cur < to
	} else {
		inWindow = cur >= from || cur < to
	}
	if !inWindow {
		return true, "当前不在允许外呼的时段"
	}
	return false, ""
}

func isoWeekday(t time.Time) int {
	w := int(t.Weekday())
	if w == 0 {
		return 7
	}
	return w
}

func parseWeekdays(raw string) (map[int]bool, bool) {
	out := make(map[int]bool)
	ok := false
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 7 {
			continue
		}
		out[n] = true
		ok = true
	}
	return out, ok
}

func parseHM(s string) (hour, min int, ok bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}
