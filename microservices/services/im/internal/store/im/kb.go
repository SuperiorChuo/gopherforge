package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
)

// KBHit 是用于 bot RAG / 引用 UI 的一条知识库片段。
type KBHit struct {
	DocumentID uint64  `json:"document_id"`
	Title      string  `json:"title"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

// SearchKnowledge 扫描共享的 ai_documents / ai_document_chunks（与 ai-service
// 同一个 PG）并做轻量关键词打分。不依赖 embedding，因此即使 embed 提供方离线，
// IM bot RAG 也能工作。
// 返回按分数降序排列的最多 topK 条命中；无有效匹配时返回空。
func (s *Store) SearchKnowledge(tenantID uint64, query string, topK int) ([]KBHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 3
	}
	tokens := knowledgeTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	// 优先命中的 chunk；没有 chunk 时回退到整篇文档。
	type row struct {
		DocumentID uint64
		Title      string
		Content    string
	}
	var rows []row
	// M1 限制扫描量。
	err := s.db.Raw(`
		SELECT c.document_id, d.title, c.content
		FROM ai_document_chunks c
		JOIN ai_documents d ON d.id = c.document_id
		WHERE d.tenant_id = ?
		ORDER BY c.id DESC
		LIMIT 500
	`, tenantID).Scan(&rows).Error
	if err != nil {
		// 表可能还不存在（ai 从未迁移）——视为没有知识库。
		if isMissingRelation(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(rows) == 0 {
		err = s.db.Raw(`
			SELECT id AS document_id, title, content
			FROM ai_documents
			WHERE tenant_id = ?
			ORDER BY id DESC
			LIMIT 100
		`, tenantID).Scan(&rows).Error
		if err != nil {
			if isMissingRelation(err) {
				return nil, nil
			}
			return nil, err
		}
	}

	type scored struct {
		hit   KBHit
		score float64
	}
	var scoredList []scored
	for _, r := range rows {
		sc := scoreKnowledge(r.Title+"\n"+r.Content, tokens, query)
		if sc <= 0 {
			continue
		}
		snippet := truncateRunes(strings.TrimSpace(r.Content), 220)
		scoredList = append(scoredList, scored{
			hit: KBHit{
				DocumentID: r.DocumentID,
				Title:      r.Title,
				Snippet:    snippet,
				Score:      sc,
			},
			score: sc,
		})
	}
	if len(scoredList) == 0 {
		return nil, nil
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].score == scoredList[j].score {
			return scoredList[i].hit.DocumentID > scoredList[j].hit.DocumentID
		}
		return scoredList[i].score > scoredList[j].score
	})
	// 按文档去重，保留最佳 chunk。
	seen := map[uint64]bool{}
	out := make([]KBHit, 0, topK)
	for _, srow := range scoredList {
		if seen[srow.hit.DocumentID] {
			continue
		}
		seen[srow.hit.DocumentID] = true
		out = append(out, srow.hit)
		if len(out) >= topK {
			break
		}
	}
	// 要求最小相对分数，避免随机的短 token 也“命中”。
	if len(out) > 0 && out[0].Score < 1.0 {
		return nil, nil
	}
	return out, nil
}

func knowledgeTokens(q string) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	add(q)
	for _, p := range strings.FieldsFunc(q, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("，。！？、；：,.!?;:()（）【】[]\"'", r)
	}) {
		if len([]rune(p)) >= 2 {
			add(p)
		}
	}
	// 无空白切分时按中文二元组处理。
	if !strings.ContainsAny(q, " \t") {
		runes := []rune(q)
		if len(runes) >= 4 {
			for i := 0; i+1 < len(runes); i++ {
				add(string(runes[i : i+2]))
			}
		}
	}
	return out
}

// scoreKnowledge：完整查询命中权重高；每个 token 命中 +1。
func scoreKnowledge(text string, tokens []string, full string) float64 {
	lower := strings.ToLower(text)
	fullL := strings.ToLower(full)
	var score float64
	if fullL != "" && strings.Contains(lower, fullL) {
		score += 5
	}
	for _, t := range tokens {
		if t == full {
			continue
		}
		if strings.Contains(lower, strings.ToLower(t)) {
			score += 1
		}
	}
	return score
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func isMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "undefined_table")
}

// BuildKBSystemPrompt 追加检索到的片段用于 RAG。
func BuildKBSystemPrompt(base string, hits []KBHit) string {
	if len(hits) == 0 {
		return base
	}
	if strings.TrimSpace(base) == "" {
		base = "你是企业在线客服助手，回答简洁、礼貌、可用中文。"
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n请优先依据下列知识库片段回答访客问题；若片段不足，请如实说明不确定，并建议转人工。不要编造业务数据。\n\n【知识库】\n")
	for i, h := range hits {
		fmt.Fprintf(&b, "\n[%d] %s\n%s\n", i+1, h.Title, h.Snippet)
	}
	return b.String()
}

// ---------- KB 未命中 ----------

func (s *Store) CreateKBMiss(m *model.KBMiss) error {
	if m.Status == "" {
		m.Status = "open"
	}
	return s.db.Create(m).Error
}

type KBMissFilter struct {
	Status  string
	Keyword string
	Page    int
	Size    int
}

func (s *Store) ListKBMisses(tenantID uint64, f KBMissFilter) ([]model.KBMiss, int64, error) {
	q := s.db.Model(&model.KBMiss{}).Where("tenant_id = ?", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("question ILIKE ? OR note ILIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := f.Page, f.Size
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	var list []model.KBMiss
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

func (s *Store) UpdateKBMiss(id, tenantID uint64, status, note *string) (*model.KBMiss, error) {
	var m model.KBMiss
	if err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&m).Error; err != nil {
		return nil, err
	}
	if status != nil && *status != "" {
		m.Status = *status
	}
	if note != nil {
		m.Note = *note
	}
	if err := s.db.Save(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) CountOpenKBMisses(tenantID uint64) (int64, error) {
	var n int64
	err := s.db.Model(&model.KBMiss{}).Where("tenant_id = ? AND status = ?", tenantID, "open").Count(&n).Error
	return n, err
}

// RecordKBSearch 递增每日计数器。source: vector | keyword | miss | vector_error
func (s *Store) RecordKBSearch(tenantID uint64, source string) {
	if tenantID == 0 {
		tenantID = 1
	}
	day := time.Now().Format("2006-01-02")
	var st model.KBDailyStat
	_ = s.db.Where(model.KBDailyStat{TenantID: tenantID, Day: day}).
		Attrs(model.KBDailyStat{TenantID: tenantID, Day: day}).
		FirstOrCreate(&st).Error
	if st.ID == 0 {
		return
	}
	updates := map[string]any{}
	switch source {
	case "vector":
		updates["hits"] = gorm.Expr("hits + 1")
		updates["vector_hits"] = gorm.Expr("vector_hits + 1")
	case "keyword":
		updates["hits"] = gorm.Expr("hits + 1")
		updates["keyword_hits"] = gorm.Expr("keyword_hits + 1")
	case "miss":
		updates["misses"] = gorm.Expr("misses + 1")
	case "vector_error":
		updates["vector_errors"] = gorm.Expr("vector_errors + 1")
	default:
		return
	}
	_ = s.db.Model(&model.KBDailyStat{}).Where("id = ?", st.ID).Updates(updates).Error
}

// KBStatsSummary 汇总最近几天数据供坐席 UI 使用。
type KBStatsSummary struct {
	Days         int                `json:"days"`
	Hits         int64              `json:"hits"`
	Misses       int64              `json:"misses"`
	VectorHits   int64              `json:"vector_hits"`
	KeywordHits  int64              `json:"keyword_hits"`
	VectorErrors int64              `json:"vector_errors"`
	HitRate      float64            `json:"hit_rate"`
	Today        *model.KBDailyStat `json:"today,omitempty"`
}

func (s *Store) KBStats(tenantID uint64, days int) (KBStatsSummary, error) {
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	since := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")
	var rows []model.KBDailyStat
	// 一天一行；days 已封顶 90，再加 Limit 防止脏数据把日报一次拉爆。
	err := s.db.Where("tenant_id = ? AND day >= ?", tenantID, since).Order("day ASC").Limit(days).Find(&rows).Error
	if err != nil {
		return KBStatsSummary{}, err
	}
	sum := KBStatsSummary{Days: days}
	today := time.Now().Format("2006-01-02")
	for _, r := range rows {
		sum.Hits += r.Hits
		sum.Misses += r.Misses
		sum.VectorHits += r.VectorHits
		sum.KeywordHits += r.KeywordHits
		sum.VectorErrors += r.VectorErrors
		if r.Day == today {
			cp := r
			sum.Today = &cp
		}
	}
	total := sum.Hits + sum.Misses
	if total > 0 {
		sum.HitRate = float64(sum.Hits) / float64(total)
	}
	return sum, nil
}
