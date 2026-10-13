package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.Brand{}, &model.Keyword{}, &model.GeoPrompt{},
		&model.GeoRun{}, &model.GeoResult{}, &model.SerpTask{},
	); err != nil {
		t.Fatal(err)
	}
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestOverviewAggregatesMatchPerRowCount 验证聚合查询结果与原逐条 COUNT 一致。
func TestOverviewAggregatesMatchPerRowCount(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	// 准备数据：2 品牌 1 竞品、8 关键词（各档位都有）、3 提示、2 运行、5 结果（3 提及）
	brands := []model.Brand{
		{TenantID: tenant, Name: "A", Kind: "brand", Status: 1},
		{TenantID: tenant, Name: "B", Kind: "brand", Status: 1},
		{TenantID: tenant, Name: "C", Kind: "competitor", Status: 1},
		{TenantID: tenant, Name: "D", Kind: "brand"}, // 下面显式置 status=0
	}
	if err := s.db.Create(&brands).Error; err != nil {
		t.Fatal(err)
	}
	// status 有 gorm default:1，赋零值会被默认值覆盖，禁用态必须建后显式改
	if err := s.db.Model(&model.Brand{}).Where("name = ?", "D").
		Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	keywords := []model.Keyword{
		{TenantID: tenant, Keyword: "tracking1", Status: "tracking", Rank: 5, BingRank: 2, BaiduRank: 8, GoogleRank: 3},
		{TenantID: tenant, Keyword: "tracking2", Status: "tracking", Rank: 0, BingRank: 0, BaiduRank: 9, GoogleRank: 0}, // norank 但 baidu 有
		{TenantID: tenant, Keyword: "tracking3", Status: "tracking", Rank: 12, BingRank: 5, BaiduRank: 0, GoogleRank: 11},
		{TenantID: tenant, Keyword: "paused", Status: "paused", Rank: 1}, // paused 不计
	}
	if err := s.db.Create(&keywords).Error; err != nil {
		t.Fatal(err)
	}
	prompts := []model.GeoPrompt{
		{TenantID: tenant, Question: "p1", Status: 1},
		{TenantID: tenant, Question: "p2", Status: 1},
		{TenantID: tenant, Question: "p3"}, // 下面显式置 status=0
	}
	if err := s.db.Create(&prompts).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&model.GeoPrompt{}).Where("question = ?", "p3").
		Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	runs := []model.GeoRun{
		{TenantID: tenant, Mode: "llm", Status: "done"},
		{TenantID: tenant, Mode: "llm", Status: "done"},
	}
	if err := s.db.Create(&runs).Error; err != nil {
		t.Fatal(err)
	}
	results := []model.GeoResult{
		{TenantID: tenant, RunID: 1, PromptID: 1, Question: "q1", Engine: "llm", MentionBrand: true},
		{TenantID: tenant, RunID: 1, PromptID: 1, Question: "q1", Engine: "llm", MentionBrand: true},
		{TenantID: tenant, RunID: 2, PromptID: 2, Question: "q2", Engine: "llm", MentionBrand: false},
		{TenantID: tenant, RunID: 2, PromptID: 2, Question: "q2", Engine: "llm", MentionBrand: true},
		{TenantID: tenant, RunID: 2, PromptID: 2, Question: "q2", Engine: "llm", MentionBrand: false},
	}
	if err := s.db.Create(&results).Error; err != nil {
		t.Fatal(err)
	}

	// 调用聚合版
	stats, err := s.Overview(tenant, 30, false)
	if err != nil {
		t.Fatal(err)
	}

	// 逐项验证：品牌/竞品
	if stats.BrandCount != 2 {
		t.Errorf("BrandCount = %d, want 2", stats.BrandCount)
	}
	if stats.CompetitorCount != 1 {
		t.Errorf("CompetitorCount = %d, want 1", stats.CompetitorCount)
	}
	// 关键词：总计 3 tracking，top10=1（rank 5），norank=1（rank 0），
	// bing10=2（rank 2 与 rank 5），baidu10=2（8+9），google10=1（rank 3）
	if stats.KeywordCount != 3 {
		t.Errorf("KeywordCount = %d, want 3", stats.KeywordCount)
	}
	if stats.KeywordTop10 != 1 {
		t.Errorf("KeywordTop10 = %d, want 1", stats.KeywordTop10)
	}
	if stats.KeywordNoRank != 1 {
		t.Errorf("KeywordNoRank = %d, want 1", stats.KeywordNoRank)
	}
	if stats.BingTop10 != 2 {
		t.Errorf("BingTop10 = %d, want 2", stats.BingTop10)
	}
	if stats.BaiduTop10 != 2 {
		t.Errorf("BaiduTop10 = %d, want 2", stats.BaiduTop10)
	}
	if stats.GoogleTop10 != 1 {
		t.Errorf("GoogleTop10 = %d, want 1", stats.GoogleTop10)
	}
	// 提示、运行、结果
	if stats.GeoPromptCount != 2 {
		t.Errorf("GeoPromptCount = %d, want 2", stats.GeoPromptCount)
	}
	if stats.GeoRunCount != 2 {
		t.Errorf("GeoRunCount = %d, want 2", stats.GeoRunCount)
	}
	if stats.GeoResultCount != 5 {
		t.Errorf("GeoResultCount = %d, want 5", stats.GeoResultCount)
	}
	if stats.GeoMentionCount != 3 {
		t.Errorf("GeoMentionCount = %d, want 3", stats.GeoMentionCount)
	}
	wantRate := 3.0 / 5.0
	if stats.GeoMentionRate < wantRate-0.01 || stats.GeoMentionRate > wantRate+0.01 {
		t.Errorf("GeoMentionRate = %f, want ~%f", stats.GeoMentionRate, wantRate)
	}
}

// TestEnsureIndexesIdempotent 索引创建幂等（PG 下才生效，sqlite 跳过）。
func TestEnsureIndexesIdempotent(t *testing.T) {
	s := newTestStore(t)
	// sqlite 直接返回，不报错即可
	if s.db.Dialector.Name() != "postgres" {
		return
	}
	// 若在 PG 下跑（CI），重复执行不报错
	if err := s.ensureIndexes(); err != nil {
		t.Fatalf("第二次 ensureIndexes 失败: %v", err)
	}
}
