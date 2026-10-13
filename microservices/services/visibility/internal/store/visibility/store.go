// Package store persists visibility (SEO/GEO) entities. AutoMigrate 实验线。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db       *gorm.DB
	idClient *identityclient.Client
}

// SetIdentity 设置 identity owner API 客户端（Phase 3：gRPC 优先 + HTTP 回退）。
func (s *Store) SetIdentity(apiBase, internalToken string) {
	// 保留 HTTP 回退配置；Consul 解析器懒建（不连接不失败）。
	if c, err := identityclient.New(apiBase, internalToken); err == nil {
		s.idClient = c
	}
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db)
}

func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := db.AutoMigrate(
		&model.Brand{},
		&model.Keyword{},
		&model.GeoPrompt{},
		&model.GeoRun{},
		&model.GeoResult{},
		&model.RankSnapshot{},
		&model.SerpTask{},
	); err != nil {
		return nil, err
	}
	if err := s.ensureIndexes(); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureIndexes 补建 gorm tag 表达不了的组合索引（列表都是 tenant_id 过滤
// + id DESC 分页，概览按 tenant+status 聚合）。IF NOT EXISTS 幂等；
// 仅 PG 执行（测试注入 sqlite）。
func (s *Store) ensureIndexes() error {
	if s.db.Dialector.Name() != "postgres" {
		return nil
	}
	for _, stmt := range indexStatements() {
		if err := s.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure index (%s): %w", stmt, err)
		}
	}
	return nil
}

// indexStatements 组合索引 DDL 清单（测试直接跑两遍验幂等）。
func indexStatements() []string {
	return []string{
		// 关键词列表分页 + 概览按 status 聚合六项指标
		`CREATE INDEX IF NOT EXISTS idx_vis_keywords_tenant_id ON vis_keywords (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_vis_keywords_tenant_status_id ON vis_keywords (tenant_id, status, id DESC)`,
		// 品牌/竞品列表与概览分组
		`CREATE INDEX IF NOT EXISTS idx_vis_brands_tenant_id ON vis_brands (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_vis_brands_tenant_status_id ON vis_brands (tenant_id, status, id DESC)`,
		// GEO 提问列表与启用档聚合
		`CREATE INDEX IF NOT EXISTS idx_vis_geo_prompts_tenant_id ON vis_geo_prompts (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_vis_geo_prompts_tenant_status_id ON vis_geo_prompts (tenant_id, status, id DESC)`,
		// 运行列表 + 概览近 N 天窗口
		`CREATE INDEX IF NOT EXISTS idx_vis_geo_runs_tenant_id ON vis_geo_runs (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_vis_geo_runs_tenant_created ON vis_geo_runs (tenant_id, created_at DESC)`,
		// 概览提及率 / 排行榜按 tenant + 时间窗扫结果
		`CREATE INDEX IF NOT EXISTS idx_vis_geo_results_tenant_created ON vis_geo_results (tenant_id, created_at DESC)`,
		// 采集任务队列（status 过滤 + 先进先出）
		`CREATE INDEX IF NOT EXISTS idx_vis_serp_tasks_tenant_status_id ON vis_serp_tasks (tenant_id, status, id DESC)`,
	}
}

func (s *Store) DB() *gorm.DB { return s.db }

func tenantQ(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

type Page struct {
	Page     int
	PageSize int
}

func (p Page) clamp() (offset, limit int) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return (p.Page - 1) * p.PageSize, p.PageSize
}

// ---------- Brands ----------

func (s *Store) ListBrands(tenantID uint64, kind string, p Page) ([]model.Brand, int64, error) {
	q := tenantQ(s.db.Model(&model.Brand{}), tenantID)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	off, lim := p.clamp()
	var list []model.Brand
	err := q.Order("kind ASC, id DESC").Offset(off).Limit(lim).Find(&list).Error
	return list, total, err
}

func (s *Store) ListAllBrands(tenantID uint64) ([]model.Brand, error) {
	var list []model.Brand
	err := tenantQ(s.db, tenantID).Where("status = ?", 1).Order("kind ASC, id ASC").Find(&list).Error
	return list, err
}

func (s *Store) CreateBrand(ctx context.Context, b *model.Brand) error {
	return s.db.WithContext(ctx).Create(b).Error
}

func (s *Store) UpdateBrand(ctx context.Context, tenantID, id uint64, updates map[string]any) error {
	res := tenantQ(s.db.WithContext(ctx).Model(&model.Brand{}), tenantID).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) DeleteBrand(ctx context.Context, tenantID, id uint64) error {
	res := tenantQ(s.db.WithContext(ctx), tenantID).Delete(&model.Brand{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) GetBrand(tenantID, id uint64) (*model.Brand, error) {
	var b model.Brand
	err := tenantQ(s.db, tenantID).First(&b, id).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ---------- Keywords (SEO) ----------

func (s *Store) ListKeywords(tenantID uint64, status, kw string, p Page) ([]model.Keyword, int64, error) {
	q := tenantQ(s.db.Model(&model.Keyword{}), tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if k := strings.TrimSpace(kw); k != "" {
		like := "%" + k + "%"
		q = q.Where("keyword LIKE ? OR target_url LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	off, lim := p.clamp()
	var list []model.Keyword
	err := q.Order("id DESC").Offset(off).Limit(lim).Find(&list).Error
	return list, total, err
}

func (s *Store) CreateKeyword(ctx context.Context, k *model.Keyword) error {
	return s.db.WithContext(ctx).Create(k).Error
}

func (s *Store) GetKeyword(tenantID, id uint64) (*model.Keyword, error) {
	var k model.Keyword
	err := tenantQ(s.db, tenantID).First(&k, id).Error
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// ListTrackingKeywords 返回待采集关键词：keywordID > 0 时只取该条（须为 tracking），否则取全部 tracking。
func (s *Store) ListTrackingKeywords(tenantID, keywordID uint64) ([]model.Keyword, error) {
	q := tenantQ(s.db, tenantID).Where("status = ?", "tracking")
	if keywordID > 0 {
		q = q.Where("id = ?", keywordID)
	}
	return fullread.All[model.Keyword](q.Order("id ASC"))
}

// ListTenantIDsWithActiveGeoPrompts 用于 GEO 周检 cron：找出所有有启用检测问题的租户。
func (s *Store) ListTenantIDsWithActiveGeoPrompts() ([]uint64, error) {
	var ids []uint64
	err := s.db.Model(&model.GeoPrompt{}).Where("status = ?", 1).
		Distinct("tenant_id").Pluck("tenant_id", &ids).Error
	return ids, err
}

// ListTenantIDsWithTrackingKeywords 用于 cron：找出所有有 tracking 关键词的租户。
func (s *Store) ListTenantIDsWithTrackingKeywords() ([]uint64, error) {
	var ids []uint64
	err := s.db.Model(&model.Keyword{}).Where("status = ?", "tracking").
		Distinct("tenant_id").Pluck("tenant_id", &ids).Error
	return ids, err
}

func (s *Store) UpdateKeyword(ctx context.Context, tenantID, id uint64, updates map[string]any) error {
	res := tenantQ(s.db.WithContext(ctx).Model(&model.Keyword{}), tenantID).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) DeleteKeyword(ctx context.Context, tenantID, id uint64) error {
	res := tenantQ(s.db.WithContext(ctx), tenantID).Delete(&model.Keyword{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ---------- Geo prompts ----------

func (s *Store) ListGeoPrompts(tenantID uint64, p Page) ([]model.GeoPrompt, int64, error) {
	q := tenantQ(s.db.Model(&model.GeoPrompt{}), tenantID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	off, lim := p.clamp()
	var list []model.GeoPrompt
	err := q.Order("id DESC").Offset(off).Limit(lim).Find(&list).Error
	return list, total, err
}

func (s *Store) ListActiveGeoPrompts(tenantID uint64) ([]model.GeoPrompt, error) {
	var list []model.GeoPrompt
	err := tenantQ(s.db, tenantID).Where("status = ?", 1).Order("id ASC").Find(&list).Error
	return list, err
}

func (s *Store) CreateGeoPrompt(ctx context.Context, p *model.GeoPrompt) error {
	return s.db.WithContext(ctx).Create(p).Error
}

// ListKeywordTexts 返回租户已有的所有关键词文本（含暂停/归档），供批量入库去重。
func (s *Store) ListKeywordTexts(tenantID uint64) ([]string, error) {
	var ks []string
	err := tenantQ(s.db.Model(&model.Keyword{}), tenantID).Pluck("keyword", &ks).Error
	return ks, err
}

// ListBrandNames 返回租户品牌库所有名称与别名（小写、去空），供批量入库去重。
func (s *Store) ListBrandNames(tenantID uint64) (map[string]bool, error) {
	brands, err := s.ListAllBrands(tenantID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(brands)*2)
	for _, b := range brands {
		seen[strings.ToLower(strings.TrimSpace(b.Name))] = true
		for _, al := range model.SplitAliases(b.Aliases) {
			seen[strings.ToLower(al)] = true
		}
	}
	return seen, nil
}

// ListGeoPromptQuestions 返回租户题库中所有问题文本（含停用），供批量入库去重。
func (s *Store) ListGeoPromptQuestions(tenantID uint64) ([]string, error) {
	var qs []string
	err := tenantQ(s.db.Model(&model.GeoPrompt{}), tenantID).Pluck("question", &qs).Error
	return qs, err
}

func (s *Store) UpdateGeoPrompt(ctx context.Context, tenantID, id uint64, updates map[string]any) error {
	res := tenantQ(s.db.WithContext(ctx).Model(&model.GeoPrompt{}), tenantID).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) DeleteGeoPrompt(ctx context.Context, tenantID, id uint64) error {
	res := tenantQ(s.db.WithContext(ctx), tenantID).Delete(&model.GeoPrompt{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ---------- Geo runs / results ----------

func (s *Store) CreateGeoRun(r *model.GeoRun) error {
	return s.db.Create(r).Error
}

func (s *Store) UpdateGeoRun(tenantID, id uint64, updates map[string]any) error {
	return tenantQ(s.db.Model(&model.GeoRun{}), tenantID).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) GetGeoRun(tenantID, id uint64) (*model.GeoRun, error) {
	var r model.GeoRun
	err := tenantQ(s.db, tenantID).First(&r, id).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListGeoRuns(tenantID uint64, p Page) ([]model.GeoRun, int64, error) {
	q := tenantQ(s.db.Model(&model.GeoRun{}), tenantID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	off, lim := p.clamp()
	var list []model.GeoRun
	err := q.Order("id DESC").Offset(off).Limit(lim).Find(&list).Error
	return list, total, err
}

func (s *Store) CreateGeoResult(r *model.GeoResult) error {
	return s.db.Create(r).Error
}

func (s *Store) ListGeoResults(tenantID, runID uint64) ([]model.GeoResult, error) {
	list, err := fullread.All[model.GeoResult](tenantQ(s.db, tenantID).Where("run_id = ?", runID).Order("id ASC"))
	if err != nil {
		return nil, err
	}
	// sources_json（DB 列）→ Sources（API 视图）：单读路径，覆盖详情/排行榜/对比/建议。
	for i := range list {
		if strings.TrimSpace(list[i].SourcesJSON) == "" {
			continue
		}
		_ = json.Unmarshal([]byte(list[i].SourcesJSON), &list[i].Sources)
	}
	return list, nil
}

// FindPreviousDoneGeoRun returns the latest done run before beforeID (same tenant).
func (s *Store) FindPreviousDoneGeoRun(tenantID, beforeID uint64) (*model.GeoRun, error) {
	var r model.GeoRun
	q := tenantQ(s.db, tenantID).Where("status = ? AND id < ?", "done", beforeID)
	err := q.Order("id DESC").First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GeoRunCompare is a delta summary between two completed runs (历史对比).
type GeoRunCompare struct {
	CurrentID        uint64   `json:"current_id"`
	PreviousID       uint64   `json:"previous_id"`
	CurrentRate      float64  `json:"current_rate"`
	PreviousRate     float64  `json:"previous_rate"`
	RateDelta        float64  `json:"rate_delta"` // percentage points
	CurrentMentions  int      `json:"current_mentions"`
	PreviousMentions int      `json:"previous_mentions"`
	MentionDelta     int      `json:"mention_delta"`
	CurrentPrompts   int      `json:"current_prompts"`
	PreviousPrompts  int      `json:"previous_prompts"`
	NewlyMentioned   []string `json:"newly_mentioned"` // questions now hit
	NewlyMissed      []string `json:"newly_missed"`    // questions lost
	StillMissing     []string `json:"still_missing"`   // both missed
	StillMentioned   int      `json:"still_mentioned"`
}

// effectiveMentions 统计有效（剔除 llm_fallback_stub 假回答）题数与提及数。
func effectiveMentions(results []model.GeoResult) (total, mentions int) {
	for _, r := range results {
		if r.Engine == "llm_fallback_stub" {
			continue
		}
		total++
		if r.MentionBrand {
			mentions++
		}
	}
	return total, mentions
}

// BuildGeoRunCompare compares current vs previous by matching question text.
// 提及率一律基于有效结果重算，与 Overview / 排行榜同口径——run 行上的
// prompt_count/mention_count 含 fallback 假回答，不可比。
func BuildGeoRunCompare(cur *model.GeoRun, curRes []model.GeoResult, prev *model.GeoRun, prevRes []model.GeoResult) GeoRunCompare {
	curTotal, curMentions := effectiveMentions(curRes)
	prevTotal, prevMentions := effectiveMentions(prevRes)
	rate := func(m, t int) float64 {
		if t <= 0 {
			return 0
		}
		return float64(m) * 100 / float64(t)
	}
	out := GeoRunCompare{
		CurrentID:        cur.ID,
		PreviousID:       prev.ID,
		CurrentRate:      rate(curMentions, curTotal),
		PreviousRate:     rate(prevMentions, prevTotal),
		CurrentMentions:  curMentions,
		PreviousMentions: prevMentions,
		CurrentPrompts:   curTotal,
		PreviousPrompts:  prevTotal,
	}
	out.RateDelta = out.CurrentRate - out.PreviousRate
	out.MentionDelta = out.CurrentMentions - out.PreviousMentions

	// 同题多行时任一提及即 true（重复题文本合并，不受行序影响）
	prevMap := map[string]bool{}
	for _, r := range prevRes {
		if r.Engine == "llm_fallback_stub" {
			continue
		}
		q := strings.TrimSpace(r.Question)
		prevMap[q] = prevMap[q] || r.MentionBrand
	}
	seen := map[string]bool{}
	for _, r := range curRes {
		if r.Engine == "llm_fallback_stub" {
			continue
		}
		q := strings.TrimSpace(r.Question)
		if seen[q] {
			continue
		}
		seen[q] = true
		was, ok := prevMap[q]
		if !ok {
			continue
		}
		if r.MentionBrand && !was {
			out.NewlyMentioned = append(out.NewlyMentioned, q)
		}
		if !r.MentionBrand && was {
			out.NewlyMissed = append(out.NewlyMissed, q)
		}
		if !r.MentionBrand && !was {
			out.StillMissing = append(out.StillMissing, q)
		}
		if r.MentionBrand && was {
			out.StillMentioned++
		}
	}
	return out
}

// FindRunningGeoRun 返回该租户当前 running 的跑题任务（无则 nil）。
func (s *Store) FindRunningGeoRun(tenantID uint64) (*model.GeoRun, error) {
	var r model.GeoRun
	err := tenantQ(s.db, tenantID).Where("status = ?", "running").Order("id DESC").First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ---------- SERP tasks / rank snapshots ----------

func (s *Store) CreateSerpTask(t *model.SerpTask) error {
	return s.db.Create(t).Error
}

func (s *Store) UpdateSerpTask(tenantID, id uint64, updates map[string]any) error {
	return tenantQ(s.db.Model(&model.SerpTask{}), tenantID).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) GetSerpTask(tenantID, id uint64) (*model.SerpTask, error) {
	var t model.SerpTask
	err := tenantQ(s.db, tenantID).First(&t, id).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListSerpTasks(tenantID uint64, p Page) ([]model.SerpTask, int64, error) {
	q := tenantQ(s.db.Model(&model.SerpTask{}), tenantID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	off, lim := p.clamp()
	var list []model.SerpTask
	err := q.Order("id DESC").Offset(off).Limit(lim).Find(&list).Error
	return list, total, err
}

// FindRunningSerpTask 返回该租户当前 running 的采集任务（无则 nil）。
func (s *Store) FindRunningSerpTask(tenantID uint64) (*model.SerpTask, error) {
	var t model.SerpTask
	err := tenantQ(s.db, tenantID).Where("status = ?", "running").Order("id DESC").First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateRankSnapshot(r *model.RankSnapshot) error {
	return s.db.Create(r).Error
}

// ListRankSnapshots 返回某关键词最近 days 天的排名历史（趋势数据）。
func (s *Store) ListRankSnapshots(tenantID, keywordID uint64, engine string, days int) ([]model.RankSnapshot, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	q := tenantQ(s.db, tenantID).Where("keyword_id = ? AND created_at >= ?", keywordID, time.Now().AddDate(0, 0, -days))
	if engine != "" {
		q = q.Where("engine = ?", engine)
	}
	return fullread.All[model.RankSnapshot](q.Order("id ASC"))
}

// EngineRank 是一次采集成功的引擎观测（失败的引擎不要传进来）。
type EngineRank struct {
	Engine string // bing | baidu | google
	Rank   int    // 0 = 采集成功但前 N 未收录
}

// ApplyEngineRanks 把成功引擎的最新排名写到 keyword 上：
// 各引擎 prev = 旧 rank、rank = 新值；汇总 rank = 三引擎非 0 最小值、prev_rank = 旧汇总 rank。
func (s *Store) ApplyEngineRanks(tenantID, keywordID uint64, ranks []EngineRank) error {
	if len(ranks) == 0 {
		return nil
	}
	// 事务 + 行锁：与用户在页面上并发编辑同一关键词时，防止基于旧快照的
	// read-modify-write 覆盖对方的更新（丢更新）
	return s.db.Transaction(func(tx *gorm.DB) error {
		var kw model.Keyword
		if err := tenantQ(tx.Clauses(clause.Locking{Strength: "UPDATE"}), tenantID).
			First(&kw, keywordID).Error; err != nil {
			return err
		}
		return applyEngineRanksLocked(tx, tenantID, &kw, ranks)
	})
}

func applyEngineRanksLocked(tx *gorm.DB, tenantID uint64, kw *model.Keyword, ranks []EngineRank) error {
	now := time.Now()
	updates := map[string]any{
		"serp_checked_at": now,
		"checked_at":      now,
	}
	best := struct{ bing, baidu, google int }{kw.BingRank, kw.BaiduRank, kw.GoogleRank}
	// 扩展引擎读旧 JSON、写新值
	extra := map[string]ExtraRank{}
	if strings.TrimSpace(kw.ExtraRanks) != "" {
		_ = json.Unmarshal([]byte(kw.ExtraRanks), &extra)
	}
	extraDirty := false
	for _, r := range ranks {
		switch r.Engine {
		case "bing":
			updates["bing_prev_rank"] = kw.BingRank
			updates["bing_rank"] = r.Rank
			best.bing = r.Rank
		case "baidu":
			updates["baidu_prev_rank"] = kw.BaiduRank
			updates["baidu_rank"] = r.Rank
			best.baidu = r.Rank
		case "google":
			updates["google_prev_rank"] = kw.GoogleRank
			updates["google_rank"] = r.Rank
			best.google = r.Rank
		default:
			prev := extra[r.Engine].Rank
			extra[r.Engine] = ExtraRank{Rank: r.Rank, PrevRank: prev}
			extraDirty = true
		}
	}
	if extraDirty {
		if raw, err := json.Marshal(extra); err == nil {
			updates["extra_ranks"] = string(raw)
		}
	}
	newBest := 0
	candidates := []int{best.bing, best.baidu, best.google}
	for _, er := range extra {
		candidates = append(candidates, er.Rank)
	}
	for _, v := range candidates {
		if v > 0 && (newBest == 0 || v < newBest) {
			newBest = v
		}
	}
	updates["prev_rank"] = kw.Rank
	updates["rank"] = newBest
	res := tenantQ(tx.Model(&model.Keyword{}), tenantID).Where("id = ?", kw.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ExtraRank 扩展引擎的一组最新/上次排名。
type ExtraRank struct {
	Rank     int `json:"rank"`
	PrevRank int `json:"prev_rank"`
}

// FailStaleTasks 启动恢复：把上次进程遗留的 running 任务全部标记为 failed（不带租户过滤，全局清理）。
func (s *Store) FailStaleTasks(reason string) error {
	now := time.Now()
	if err := s.db.Model(&model.GeoRun{}).Where("status = ?", "running").
		Updates(map[string]any{"status": "failed", "error": reason, "finished_at": now}).Error; err != nil {
		return err
	}
	return s.db.Model(&model.SerpTask{}).Where("status = ?", "running").
		Updates(map[string]any{"status": "failed", "error": reason, "finished_at": now}).Error
}

// ListAdminUserIDs 返回租户内启用的管理员用户 id（GEO 周报收件人）。
// Phase 2C：改走 identity owner API（/internal/users/admin-ids），不再直查共享 users 表。
// identity 不可达时软读降级（返回空列表，周报不误发）。
func (s *Store) ListAdminUserIDs(ctx context.Context, tenantID uint64) ([]uint64, error) {
	// Phase 3：经共享 identityclient（gRPC 优先 + HTTP 回退）。未配置 → 空列表（不误发）。
	if s.idClient == nil {
		return nil, nil
	}
	ids, err := s.idClient.AdminUserIDs(ctx, tenantID)
	if err != nil {
		log.Printf("visibility: identity admin-ids skipped (tenant=%d): %v", tenantID, err)
		return nil, nil
	}
	return ids, nil
}

// GetSettingJSON 读 system_settings 表某行的 value_json（控制台热配置，全局共享表，非本服务所有）。
// 行不存在返回 (nil, nil)。
func (s *Store) GetSettingJSON(key string) (map[string]any, error) {
	var raw string
	err := s.db.Table("system_settings").Where("setting_key = ?", key).
		Select("value_json").Limit(1).Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- Stats ----------

type OverviewStats struct {
	BrandCount      int64   `json:"brand_count"`
	CompetitorCount int64   `json:"competitor_count"`
	KeywordCount    int64   `json:"keyword_count"`
	KeywordTop10    int64   `json:"keyword_top10"`
	KeywordNoRank   int64   `json:"keyword_no_rank"`
	BingTop10       int64   `json:"bing_top10"`
	BaiduTop10      int64   `json:"baidu_top10"`
	GoogleTop10     int64   `json:"google_top10"`
	GeoPromptCount  int64   `json:"geo_prompt_count"`
	GeoRunCount     int64   `json:"geo_run_count"`
	GeoResultCount  int64   `json:"geo_result_count"`
	GeoMentionCount int64   `json:"geo_mention_count"`
	GeoMentionRate  float64 `json:"geo_mention_rate"`
	RecentRuns      int64   `json:"recent_runs"`
}

// Overview 租户概览。allowDemoData 与 GeoLeaderboard 同义：只有演示模式为 true。
// 关时 stub 演示行一律不计入提及率——它们的模板必含自有品牌名、提及恒 100%，
// 混进"近 30 天 AI 提及率"就是把假数据当经营指标看。
func (s *Store) Overview(tenantID uint64, days int, allowDemoData bool) (*OverviewStats, error) {
	if days <= 0 {
		days = 30
	}
	out := &OverviewStats{}
	// 品牌/竞品：同表两次 COUNT 合一条分组聚合
	type brandRow struct {
		Kind string
		Cnt  int64
	}
	var brows []brandRow
	if err := tenantQ(s.db.Model(&model.Brand{}), tenantID).
		Where("kind IN ? AND status = ?", []string{"brand", "competitor"}, 1).
		Select("kind, COUNT(*) AS cnt").Group("kind").Scan(&brows).Error; err == nil {
		for _, r := range brows {
			switch r.Kind {
			case "brand":
				out.BrandCount = r.Cnt
			case "competitor":
				out.CompetitorCount = r.Cnt
			}
		}
	}
	// 关键词六项指标合一条：SUM(CASE WHEN ...) 而不是 count(*) FILTER，
	// 后者 sqlite 要 3.30+，测试驱动（modernc）版本不做保证，兼容写法零风险。
	var kw struct {
		Total    int64
		Top10    int64
		NoRank   int64
		Bing10   int64
		Baidu10  int64
		Google10 int64
	}
	_ = tenantQ(s.db.Model(&model.Keyword{}), tenantID).Where("status = ?", "tracking").
		Select(`COUNT(*) AS total,
			SUM(CASE WHEN rank > 0 AND rank <= 10 THEN 1 ELSE 0 END) AS top10,
			SUM(CASE WHEN rank = 0 THEN 1 ELSE 0 END) AS no_rank,
			SUM(CASE WHEN bing_rank > 0 AND bing_rank <= 10 THEN 1 ELSE 0 END) AS bing10,
			SUM(CASE WHEN baidu_rank > 0 AND baidu_rank <= 10 THEN 1 ELSE 0 END) AS baidu10,
			SUM(CASE WHEN google_rank > 0 AND google_rank <= 10 THEN 1 ELSE 0 END) AS google10`).
		Scan(&kw).Error
	out.KeywordCount, out.KeywordTop10, out.KeywordNoRank = kw.Total, kw.Top10, kw.NoRank
	out.BingTop10, out.BaiduTop10, out.GoogleTop10 = kw.Bing10, kw.Baidu10, kw.Google10
	_ = tenantQ(s.db.Model(&model.GeoPrompt{}), tenantID).Where("status = ?", 1).Count(&out.GeoPromptCount).Error

	since := time.Now().AddDate(0, 0, -days)
	// 运行数：总量 + 近 N 天合一条
	var runs struct {
		Total  int64
		Recent int64
	}
	_ = tenantQ(s.db.Model(&model.GeoRun{}), tenantID).
		Select("COUNT(*) AS total, SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS recent", since).
		Scan(&runs).Error
	out.GeoRunCount, out.RecentRuns = runs.Total, runs.Recent

	// llm_fallback_stub（模型调用失败的占位行）永远剔除。
	// stub 演示行：非演示模式一律剔除；演示模式下沿用旧口径——有真实 llm 结果
	// 时才剔除，否则演示站的概览会全是 0。与排行榜口径一致。
	effective := tenantQ(s.db.Model(&model.GeoResult{}), tenantID).
		Where("created_at >= ? AND (engine IS NULL OR engine <> ?)", since, "llm_fallback_stub")
	dropStub := true
	if allowDemoData {
		// 存在任意真实引擎（llm 历史值 / chatgpt / selfhost / 预留 perplexity/gemini）
		// 行时才剔除 demo stub 行；否则演示站的概览会全是 0。与排行榜口径一致。
		var realCount int64
		_ = tenantQ(s.db.Model(&model.GeoResult{}), tenantID).
			Where("created_at >= ? AND engine IN ?", since,
				[]string{"llm", "chatgpt", "selfhost", "perplexity", "gemini"}).Count(&realCount).Error
		dropStub = realCount > 0
	}
	if dropStub {
		effective = effective.Where("engine IS NULL OR engine <> ?", "stub")
	}
	// 结果总数与提及数同口径同一批行，合一条
	var geo struct {
		Total   int64
		Mention int64
	}
	_ = effective.Session(&gorm.Session{}).
		Select("COUNT(*) AS total, SUM(CASE WHEN mention_brand = ? THEN 1 ELSE 0 END) AS mention", true).
		Scan(&geo).Error
	out.GeoResultCount, out.GeoMentionCount = geo.Total, geo.Mention
	if out.GeoResultCount > 0 {
		out.GeoMentionRate = float64(out.GeoMentionCount) / float64(out.GeoResultCount)
	}
	return out, nil
}

var ErrNotFound = errors.New("not found")
