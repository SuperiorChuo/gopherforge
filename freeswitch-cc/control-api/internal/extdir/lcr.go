package extdir

import "github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"

// LCRPreviewRow 是选路预览的一行：命中该组的一条网关及其在 failover 序列里的次序。
type LCRPreviewRow struct {
	Order      int    `json:"order"`        // 1=首选（最先尝试）
	Gateway    string `json:"gateway"`      // 网关名
	RatePerMin int64  `json:"rate_per_min"` // 分/分钟，0=未设
	Prepend    string `json:"prepend"`      // 送号前缀（去冠后再加）
}

// LCRPreview 纯内存模拟选路：在 number 前缀里找匹配的最长 DialPrefix 组，返回该组
// 按 LCR 规则排好序的 failover 序列。无匹配（无对应字冠或号码太短）返回 nil。
// 不打 FS，仅供管理面「选路预览」用。
func LCRPreview(all []store.Gateway, number string) []LCRPreviewRow {
	// 分组并挑出 number 能命中的最长字冠（非空字冠才参与选路）。
	groups := map[string][]store.Gateway{}
	for _, g := range all {
		if g.DialPrefix == "" {
			continue
		}
		groups[g.DialPrefix] = append(groups[g.DialPrefix], g)
	}
	bestPrefix := ""
	for prefix := range groups {
		if !hasPrefix(number, prefix) {
			continue
		}
		// 去冠后至少要还剩号码（拨号计划 expression 要求 \d{3,20}）。
		if len(number)-len(prefix) < 1 {
			continue
		}
		if len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix == "" {
		return nil
	}
	gws := append([]store.Gateway(nil), groups[bestPrefix]...)
	sortByRate(gws)
	rows := make([]LCRPreviewRow, 0, len(gws))
	for i, g := range gws {
		rows = append(rows, LCRPreviewRow{
			Order:      i + 1,
			Gateway:    g.Name,
			RatePerMin: g.RatePerMin,
			Prepend:    g.Prepend,
		})
	}
	return rows
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
