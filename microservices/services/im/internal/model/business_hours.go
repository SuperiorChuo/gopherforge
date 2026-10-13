package model

// BusinessHours 配置（M7）：站点级工作时间表，存 Site.BusinessHours JSON。

import (
	"encoding/json"
	"strings"
	"time"
)

type BusinessHoursConfig struct {
	Enabled bool   `json:"enabled"`
	Days    []int  `json:"days"`  // 1=周一 … 7=周日
	Start   string `json:"start"` // "09:00"
	End     string `json:"end"`   // "18:00"
	OffText string `json:"off_text,omitempty"`
}

// ParseBusinessHours 解析站点 JSON；空或损坏的输入 → 视为关闭（disabled）。
func ParseBusinessHours(raw string) BusinessHoursConfig {
	var cfg BusinessHoursConfig
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}

// WithinAt 报告 t 是否落在工作时间内。未启用配置时始终视为“在工作时间内”
// （24/7）。支持跨夜时段（22:00-06:00）。
func (c BusinessHoursConfig) WithinAt(t time.Time) bool {
	if !c.Enabled {
		return true
	}
	day := int(t.Weekday())
	if day == 0 {
		day = 7 // Go 中 Sunday=0 → 约定为 7
	}
	dayOK := len(c.Days) == 0
	for _, d := range c.Days {
		if d == day {
			dayOK = true
			break
		}
	}
	if !dayOK {
		return false
	}
	start, err1 := parseHM(c.Start)
	end, err2 := parseHM(c.End)
	if err1 != nil || err2 != nil || start == end {
		return true // 配置不完整视为全天
	}
	now := t.Hour()*60 + t.Minute()
	if start < end {
		return now >= start && now < end
	}
	// 跨夜时段
	return now >= start || now < end
}

func (c BusinessHoursConfig) OffNotice() string {
	if strings.TrimSpace(c.OffText) != "" {
		return c.OffText
	}
	return "当前为非工作时间，您可以留言，我们上班后会尽快回复。"
}

func parseHM(s string) (int, error) {
	tm, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return tm.Hour()*60 + tm.Minute(), nil
}
