package store

// GEO「AI 推荐排行榜」：聚合最近几次已完成检测的 matched_brands，
// 得出每个品牌被 AI 提到的次数与自有品牌的排名/位次表现。
// 纯 DB 统计，不调模型。

import (
	"math"
	"sort"
	"strings"

	"github.com/go-admin-kit/services/visibility/internal/engine"
	"github.com/go-admin-kit/services/visibility/internal/model"
)

// GeoLeaderboardItem 一个品牌在最近检测中的被提及统计。
type GeoLeaderboardItem struct {
	Name string `json:"name"`
	// Kind: brand | competitor | other（回答里出现但不在品牌库的名字不会进 matched_brands，故 other 罕见）
	Kind     string `json:"kind"`
	Mentions int    `json:"mentions"`
}

// GeoLeaderboardStats 排行榜聚合结果。
type GeoLeaderboardStats struct {
	RunsUsed int `json:"runs_used"`
	// Mode: llm（真实检测）｜ stub（演示模式下、且没有任何真实检测时的演示数据，
	// 前端必须标注）。生产环境不会出现 stub。
	Mode           string               `json:"mode"`
	TotalQuestions int                  `json:"total_questions"`
	Items          []GeoLeaderboardItem `json:"items"`
	OwnName        string               `json:"own_name"`
	// OwnRank: 自有品牌在榜单中的名次，0 = 未上榜（一次都没被提到）
	OwnRank     int `json:"own_rank"`
	OwnMentions int `json:"own_mentions"`
	// MentionRate: 自有品牌提及率（百分比整数）
	MentionRate int `json:"mention_rate"`
	// AvgPosition: 回答给出推荐列表时自有品牌的平均位次（1 位小数），0 = 无位次数据
	AvgPosition  float64 `json:"avg_position"`
	BestPosition int     `json:"best_position"`
}

// GeoLeaderboard 聚合最近 maxRuns 次已完成的【真实 llm 检测】。
//
// allowDemoFallback 只在演示模式（VISIBILITY_DEMO_MODE）下为 true：此时没有真实
// 检测才退回 stub 演示检测并把 Mode 标成 stub。生产环境一律 false——没有真实
// 检测就返回 RunsUsed=0 的空榜，由前端显示"还没有完成的检测"，绝不编一份榜单
// 出来靠一个 Mode 字段和前端自觉去免责。
func (s *Store) GeoLeaderboard(tenantID uint64, maxRuns int, allowDemoFallback bool) (*GeoLeaderboardStats, error) {
	if maxRuns <= 0 {
		maxRuns = 3
	}
	var runs []model.GeoRun
	if err := tenantQ(s.db, tenantID).Where("status = ? AND mode = ?", "done", "llm").
		Order("id DESC").Limit(maxRuns).Find(&runs).Error; err != nil {
		return nil, err
	}
	mode := "llm"
	if len(runs) == 0 && allowDemoFallback {
		if err := tenantQ(s.db, tenantID).Where("status = ?", "done").
			Order("id DESC").Limit(maxRuns).Find(&runs).Error; err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			mode = "stub"
		}
	}
	st := &GeoLeaderboardStats{RunsUsed: len(runs), Mode: mode, Items: []GeoLeaderboardItem{}}
	brands, err := s.ListAllBrands(tenantID)
	if err != nil {
		return nil, err
	}
	kindOf := map[string]string{}
	// 别名/大小写变体归一到标准名：历史结果里 matched_brands 可能存的是当时的写法
	stdName := map[string]string{}
	namesOf := map[string][]string{}
	for _, b := range brands {
		kindOf[b.Name] = b.Kind
		stdName[strings.ToLower(strings.TrimSpace(b.Name))] = b.Name
		namesOf[b.Name] = append(namesOf[b.Name], b.Name)
		for _, al := range model.SplitAliases(b.Aliases) {
			stdName[strings.ToLower(al)] = b.Name
			namesOf[b.Name] = append(namesOf[b.Name], al)
		}
	}
	if len(runs) == 0 {
		// 连检测都没有：仍给出自有品牌名，前端好引导
		for _, b := range brands {
			if b.Kind != "competitor" {
				st.OwnName = b.Name
				break
			}
		}
		return st, nil
	}

	counts := map[string]int{}
	ownMention, posSum, posN, best := 0, 0, 0, 0
	for _, r := range runs {
		results, err := s.ListGeoResults(tenantID, r.ID)
		if err != nil {
			return nil, err
		}
		for _, res := range results {
			// fallback 假回答的模板必然含自有品牌名，历史行一并剔除
			if res.Engine == "llm_fallback_stub" {
				continue
			}
			st.TotalQuestions++
			// 历史行经字面复核：品牌名/别名真实出现在回答里才计数
			// （老数据是修复前的宽松分析器写的，可能把 go-admin 记成了 go-admin-kit）
			verify := func(name string) bool {
				for _, n := range namesOf[name] {
					if engine.MentionedLiterally(res.Answer, n) {
						return true
					}
				}
				return len(namesOf[name]) == 0 // 不在品牌库的名字无从核验，保留原判
			}
			ownHit := false
			// 同行「标准名+别名」归一后是同一品牌，逐行去重防重复计数
			rowSeen := map[string]bool{}
			for _, m := range strings.Split(res.MatchedBrands, ",") {
				t := strings.TrimSpace(m)
				if t == "" {
					continue
				}
				if std, ok := stdName[strings.ToLower(t)]; ok {
					t = std
				}
				if rowSeen[t] || !verify(t) {
					continue
				}
				rowSeen[t] = true
				counts[t]++
				if kindOf[t] != "" && kindOf[t] != "competitor" {
					ownHit = true
				}
			}
			if ownHit {
				ownMention++
				if res.ListPosition > 0 {
					posSum += res.ListPosition
					posN++
					if best == 0 || res.ListPosition < best {
						best = res.ListPosition
					}
				}
			}
		}
	}
	for name, c := range counts {
		kind := kindOf[name]
		if kind == "" {
			kind = "other"
		}
		st.Items = append(st.Items, GeoLeaderboardItem{Name: name, Kind: kind, Mentions: c})
	}
	sort.Slice(st.Items, func(i, j int) bool {
		if st.Items[i].Mentions != st.Items[j].Mentions {
			return st.Items[i].Mentions > st.Items[j].Mentions
		}
		return st.Items[i].Name < st.Items[j].Name
	})
	// 自有品牌可能有多个：取榜上最靠前的那个作为"你的排名"
	for i, it := range st.Items {
		if it.Kind == "brand" {
			st.OwnName = it.Name
			st.OwnRank = i + 1
			st.OwnMentions = it.Mentions
			break
		}
	}
	if st.OwnName == "" {
		for _, b := range brands {
			if b.Kind != "competitor" {
				st.OwnName = b.Name
				break
			}
		}
	}
	if st.TotalQuestions > 0 {
		st.MentionRate = ownMention * 100 / st.TotalQuestions
	}
	if posN > 0 {
		st.AvgPosition = math.Round(float64(posSum)/float64(posN)*10) / 10
	}
	st.BestPosition = best
	return st, nil
}
