package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
)

func TestKnowledgeTokensAndScore(t *testing.T) {
	toks := knowledgeTokens("如何退款 订单")
	if len(toks) < 2 {
		t.Fatalf("tokens=%v", toks)
	}
	sc := scoreKnowledge("退款说明：在订单详情申请退款", toks, "如何退款")
	if sc < 1 {
		t.Fatalf("score=%v", sc)
	}
	if scoreKnowledge("无关内容", toks, "如何退款") > 0 && scoreKnowledge("无关内容", []string{"如何退款"}, "如何退款") != 0 {
		// 完整查询不在文本中，tokens 可能命中也可能不命中
	}
	if scoreKnowledge("无关内容", []string{"xyzabc"}, "xyzabc") != 0 {
		t.Fatal("expected zero")
	}
}

func TestBuildKBSystemPrompt(t *testing.T) {
	p := BuildKBSystemPrompt("你是助手", []KBHit{{Title: "退款", Snippet: "三天内可退", Score: 5}})
	if p == "你是助手" || p == "" {
		t.Fatal(p)
	}
}

func TestKBStatsCapsAtRequestedDays(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	now := time.Now()
	for i := 0; i < 10; i++ {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		if err := db.Create(&model.KBDailyStat{TenantID: 1, Day: day, Hits: 1, Misses: 1}).Error; err != nil {
			t.Fatalf("seed day %s: %v", day, err)
		}
	}

	sum, err := s.KBStats(1, 7)
	if err != nil {
		t.Fatalf("KBStats: %v", err)
	}
	if sum.Days != 7 {
		t.Fatalf("days = %d, want 7", sum.Days)
	}
	var loaded int64
	if err := db.Raw(`SELECT COUNT(*) FROM im_kb_daily_stats WHERE tenant_id = 1 AND day >= ?`, now.AddDate(0, 0, -6).Format("2006-01-02")).Scan(&loaded).Error; err != nil {
		t.Fatalf("count window: %v", err)
	}
	if loaded < 7 {
		t.Fatalf("seeded window rows = %d, want at least 7", loaded)
	}
	if sum.Hits != 7 || sum.Misses != 7 {
		t.Fatalf("hits=%d misses=%d, want Limit(days)=7 rows", sum.Hits, sum.Misses)
	}
}
