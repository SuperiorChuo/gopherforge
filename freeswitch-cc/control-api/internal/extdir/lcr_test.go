package extdir

import (
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// 选路预览：命中最长字冠组，按费率升序返回 failover 序列。
func TestLCRPreviewLongestPrefixAndOrder(t *testing.T) {
	all := []store.Gateway{
		{Name: "domestic", DialPrefix: "0", RatePerMin: 50},
		{Name: "intl_cheap", DialPrefix: "00", RatePerMin: 8},
		{Name: "intl_backup", DialPrefix: "00", RatePerMin: 20},
	}
	// 00 前缀更长，应命中国际组而非国内组。
	rows := LCRPreview(all, "0086138")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for longest prefix 00, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Gateway != "intl_cheap" || rows[0].Order != 1 {
		t.Errorf("first should be cheapest intl_cheap order1, got %+v", rows[0])
	}
	if rows[1].Gateway != "intl_backup" || rows[1].Order != 2 {
		t.Errorf("second should be intl_backup order2, got %+v", rows[1])
	}
}

// 无匹配字冠返回 nil。
func TestLCRPreviewNoMatch(t *testing.T) {
	all := []store.Gateway{{Name: "gw", DialPrefix: "9", RatePerMin: 10}}
	if rows := LCRPreview(all, "13800138000"); rows != nil {
		t.Errorf("no prefix match should be nil, got %+v", rows)
	}
}

// 空字冠网关不参与选路。
func TestLCRPreviewIgnoresEmptyPrefix(t *testing.T) {
	all := []store.Gateway{{Name: "peer", DialPrefix: "", RatePerMin: 1}}
	if rows := LCRPreview(all, "12345"); rows != nil {
		t.Errorf("empty-prefix gateway must not route, got %+v", rows)
	}
}
