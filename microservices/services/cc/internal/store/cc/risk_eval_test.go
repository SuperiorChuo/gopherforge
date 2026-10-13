package store

import (
	"testing"
	"time"
)

func cst(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, AsiaShanghai())
}

func TestEvaluateOutboundDefaultAllow(t *testing.T) {
	d := EvaluateOutbound(cst(2026, 8, 27, 10, 0), nil, OutboundRiskInput{Callee: "13800138000", FromExt: "1000"})
	if !d.Allow {
		t.Fatalf("empty policy must allow, got %+v", d)
	}
}

func TestEvaluateOutboundBlacklistExact(t *testing.T) {
	in := OutboundRiskInput{
		Callee: "13800138000",
		Lists:  []CCRiskListEntry{{Number: "13800138000", Match: "exact", ListType: "black"}},
	}
	d := EvaluateOutbound(cst(2026, 8, 27, 10, 0), nil, in)
	if d.Allow || d.Code != "blacklist" || d.Status != 403 {
		t.Fatalf("exact black: %+v", d)
	}
}

func TestEvaluateOutboundBlacklistPrefix(t *testing.T) {
	in := OutboundRiskInput{
		Callee: "0013800138000",
		Lists:  []CCRiskListEntry{{Number: "00", Match: "prefix", ListType: "black"}},
	}
	d := EvaluateOutbound(cst(2026, 8, 27, 10, 0), nil, in)
	if d.Allow || d.Code != "blacklist" {
		t.Fatalf("prefix black: %+v", d)
	}
}

func TestEvaluateOutboundWhitelistOverridesBlack(t *testing.T) {
	in := OutboundRiskInput{
		Callee: "13800138000",
		Policy: CCRiskPolicy{MaxCallsPerNumberPerHour: 1},
		Counts: OutboundRiskCounts{NumberLastHour: 9},
		Lists: []CCRiskListEntry{
			{Number: "13800138000", Match: "exact", ListType: "black"},
			{Number: "138", Match: "prefix", ListType: "white"},
		},
	}
	d := EvaluateOutbound(cst(2026, 8, 27, 23, 0), nil, in)
	if !d.Allow {
		t.Fatalf("white must skip black/freq/time, got %+v", d)
	}
}

func TestEvaluateOutboundTimeWindowWeekday(t *testing.T) {
	// 2026-08-27 是周四（ISO 4）；策略只允许工作日 1-5，周五 22 点应拒，周四 10 点放行。
	p := CCRiskPolicy{OutboundStart: "09:00", OutboundEnd: "21:00", OutboundWeekdays: "1,2,3,4,5"}
	if d := EvaluateOutbound(cst(2026, 8, 27, 10, 0), nil, OutboundRiskInput{Callee: "1", Policy: p}); !d.Allow {
		t.Fatalf("thu 10:00 allow, got %+v", d)
	}
	if d := EvaluateOutbound(cst(2026, 8, 27, 22, 0), nil, OutboundRiskInput{Callee: "1", Policy: p}); d.Allow || d.Code != "time_window" {
		t.Fatalf("thu 22:00 deny, got %+v", d)
	}
	sat := cst(2026, 8, 29, 10, 0) // 周六
	if d := EvaluateOutbound(sat, nil, OutboundRiskInput{Callee: "1", Policy: p}); d.Allow || d.Code != "time_window" {
		t.Fatalf("sat deny, got %+v", d)
	}
}

func TestEvaluateOutboundOvernightWindow(t *testing.T) {
	p := CCRiskPolicy{OutboundStart: "22:00", OutboundEnd: "06:00"}
	if d := EvaluateOutbound(cst(2026, 8, 27, 23, 0), nil, OutboundRiskInput{Callee: "1", Policy: p}); !d.Allow {
		t.Fatalf("23:00 in overnight window, got %+v", d)
	}
	if d := EvaluateOutbound(cst(2026, 8, 27, 5, 0), nil, OutboundRiskInput{Callee: "1", Policy: p}); !d.Allow {
		t.Fatalf("05:00 in overnight window, got %+v", d)
	}
	if d := EvaluateOutbound(cst(2026, 8, 27, 8, 0), nil, OutboundRiskInput{Callee: "1", Policy: p}); d.Allow {
		t.Fatalf("08:00 outside overnight window, got %+v", d)
	}
}

func TestEvaluateOutboundIncompleteTimeIgnored(t *testing.T) {
	p := CCRiskPolicy{OutboundStart: "09:00"} // 缺结束
	d := EvaluateOutbound(cst(2026, 8, 27, 23, 0), nil, OutboundRiskInput{Callee: "1", Policy: p})
	if !d.Allow {
		t.Fatalf("incomplete time must not deny, got %+v", d)
	}
}

func TestEvaluateOutboundBlockedPrefixAndRateCap(t *testing.T) {
	p := CCRiskPolicy{BlockedPrefixes: "00,166", MaxRatePerMin: 100}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "00861000", Policy: p}); d.Allow || d.Code != "blocked_prefix" {
		t.Fatalf("00 prefix: %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1380013", Policy: p, RatePerMin: 200}); d.Allow || d.Code != "rate_cap" {
		t.Fatalf("rate cap: %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1380013", Policy: p, RatePerMin: 50}); !d.Allow {
		t.Fatalf("under cap must allow, got %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1380013", Policy: p, RatePerMin: 0}); !d.Allow {
		t.Fatalf("no rate match must not hit cap, got %+v", d)
	}
}

func TestEvaluateOutboundFrequency(t *testing.T) {
	p := CCRiskPolicy{MaxCallsPerNumberPerHour: 2, MaxCallsPerAgentPerHour: 3, MaxCallsPerTenantPerMin: 1}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1", FromExt: "1000", Policy: p, Counts: OutboundRiskCounts{NumberLastHour: 2}}); d.Allow || d.Status != 429 || d.Code != "frequency_number" {
		t.Fatalf("number freq: %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1", FromExt: "1000", Policy: p, Counts: OutboundRiskCounts{AgentLastHour: 3}}); d.Allow || d.Code != "frequency_agent" {
		t.Fatalf("agent freq: %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1", FromExt: "1000", Policy: p, Counts: OutboundRiskCounts{TenantLastMin: 1}}); d.Allow || d.Code != "frequency_tenant" {
		t.Fatalf("tenant freq: %+v", d)
	}
	if d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{Callee: "1", FromExt: "1000", Policy: p, Counts: OutboundRiskCounts{NumberLastHour: 1, AgentLastHour: 2, TenantLastMin: 0}}); !d.Allow {
		t.Fatalf("under limits must allow, got %+v", d)
	}
}

func TestEvaluateOutboundZeroLimitsUnlimited(t *testing.T) {
	d := EvaluateOutbound(time.Now(), nil, OutboundRiskInput{
		Callee: "1", Policy: CCRiskPolicy{}, Counts: OutboundRiskCounts{NumberLastHour: 99, AgentLastHour: 99, TenantLastMin: 99},
	})
	if !d.Allow {
		t.Fatalf("zero limits must be unlimited, got %+v", d)
	}
}
