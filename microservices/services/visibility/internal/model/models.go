package model

import (
	"strings"
	"time"
)

// SplitAliases 拆逗号分隔的别名串：兼容全角逗号/顿号（中文输入与模型输出常见），去空。
func SplitAliases(aliases string) []string {
	r := strings.NewReplacer("，", ",", "、", ",")
	var out []string
	for _, p := range strings.Split(r.Replace(aliases), ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Brand is a owned brand or competitor under a tenant.
// Kind: brand | competitor
type Brand struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Aliases   string    `gorm:"type:text" json:"aliases,omitempty"` // comma-separated alt names
	Kind      string    `gorm:"size:32;not null;default:brand;index" json:"kind"`
	Website   string    `gorm:"size:512" json:"website,omitempty"`
	Note      string    `gorm:"type:text" json:"note,omitempty"`
	Status    int16     `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Brand) TableName() string { return "vis_brands" }

// Keyword is an SEO keyword tracking row (manual or imported).
type Keyword struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	Keyword   string `gorm:"size:256;not null" json:"keyword"`
	Locale    string `gorm:"size:32;default:zh-CN" json:"locale"`
	TargetURL string `gorm:"size:1024" json:"target_url,omitempty"`
	// Rank: 0 = not in top N / unknown; 1-100 typical.
	Rank         int `gorm:"default:0" json:"rank"`
	PrevRank     int `gorm:"default:0" json:"prev_rank"`
	SearchVolume int `gorm:"default:0" json:"search_volume"`
	// Status: tracking | paused | archived
	Status    string     `gorm:"size:32;not null;default:tracking;index" json:"status"`
	Note      string     `gorm:"type:text" json:"note,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	// Per-engine latest / previous ranks from SERP checks. 0 = 未收录或未采集。
	// 采集失败的引擎不更新字段（保留上次有效值），失败记录在 vis_rank_snapshots.error。
	BingRank       int `gorm:"default:0" json:"bing_rank"`
	BingPrevRank   int `gorm:"default:0" json:"bing_prev_rank"`
	BaiduRank      int `gorm:"default:0" json:"baidu_rank"`
	BaiduPrevRank  int `gorm:"default:0" json:"baidu_prev_rank"`
	GoogleRank     int `gorm:"default:0" json:"google_rank"`
	GooglePrevRank int `gorm:"default:0" json:"google_prev_rank"`
	// ExtraRanks: 扩展引擎（sogou/so360/duckduckgo/github…）最新排名，
	// JSON 形如 {"sogou":{"rank":3,"prev_rank":5}}；三大引擎仍走专用列。
	ExtraRanks    string     `gorm:"type:text" json:"extra_ranks,omitempty"`
	SerpCheckedAt *time.Time `json:"serp_checked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (Keyword) TableName() string { return "vis_keywords" }

// GeoPrompt is a question asked to generative engines (GEO).
type GeoPrompt struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// Category: category | comparison | scenario | other
	Category string `gorm:"size:32;default:category" json:"category"`
	Question string `gorm:"type:text;not null" json:"question"`
	// Engine hint: llm | openai_compat | stub | manual。schema 默认值必须是
	// 真实引擎——默认成 stub 等于把"不写这个字段"翻译成"跑假数据"。
	Engine    string    `gorm:"size:32;default:llm" json:"engine"`
	Status    int16     `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (GeoPrompt) TableName() string { return "vis_geo_prompts" }

// GeoRun is one batch execution of prompts.
type GeoRun struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// Mode: llm（真实模型）| stub（演示模拟，仅演示模式可创建）| manual。
	// 默认 llm：不显式选择时只能是真实检测，绝不默认落到假数据上。
	Mode string `gorm:"size:32;not null;default:llm" json:"mode"`
	// Status: running | done | failed
	Status string `gorm:"size:32;not null;default:running;index" json:"status"`
	// Engine: 本次检测使用的生成式引擎。chatgpt（真实 ChatGPT 联网搜索）|
	// selfhost（自建 bing 接地检索）| stub（演示模拟，仅演示模式可创建）。
	// 老 run 该列为空，视为 selfhost。Mode 保持 llm 不变（现有门控依赖它）。
	Engine       string `gorm:"size:32;default:selfhost;index" json:"engine"`
	PromptCount  int    `gorm:"default:0" json:"prompt_count"`
	DoneCount    int    `gorm:"default:0" json:"done_count"`
	MentionCount int    `gorm:"default:0" json:"mention_count"`
	Note         string `gorm:"size:512" json:"note,omitempty"`
	Error        string `gorm:"size:512" json:"error,omitempty"`
	// Advice: 检测完成后按需生成的 AI 优化建议（markdown），空 = 未生成
	Advice string `gorm:"type:text" json:"advice,omitempty"`
	// AdviceItemsJSON: 可执行清单 JSON 数组（[]AdviceItem），与 advice 一起生成/更新
	AdviceItemsJSON string     `gorm:"type:text" json:"advice_items_json,omitempty"`
	CreatedBy       uint64     `gorm:"default:0" json:"created_by"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AdviceItem is one actionable GEO checklist entry (stored as JSON on GeoRun).
type AdviceItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Priority string `json:"priority,omitempty"` // high | medium | low
	Related  string `json:"related,omitempty"`  // 关联问题摘要
	Done     bool   `json:"done"`
}

func (GeoRun) TableName() string { return "vis_geo_runs" }

// GeoSource is one retrieved web source injected as grounding context for an
// answer, plus whether the answer actually cited it and whether it belongs to
// an owned brand domain.
type GeoSource struct {
	Position int    `json:"position"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url"`
	// Cited: 回答实际引用了该来源（[n] 标记或字面 URL 命中）
	Cited bool `json:"cited"`
	// OwnDomain: 命中自有品牌域名（host+path，GitHub/Gitee 共享域名感知）
	OwnDomain bool `json:"own_domain"`
	// Text: 抓取到的可读正文，仅用于拼接地 prompt，不落库、不出 API
	Text string `json:"-"`
}

// GeoResult is one prompt answer within a run.
type GeoResult struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	RunID    uint64 `gorm:"not null;index" json:"run_id"`
	PromptID uint64 `gorm:"not null;index" json:"prompt_id"`
	Question string `gorm:"type:text;not null" json:"question"`
	Answer   string `gorm:"type:text" json:"answer"`
	// MentionBrand: own brand mentioned
	MentionBrand bool `gorm:"default:false;index" json:"mention_brand"`
	// MentionCompetitor: any competitor mentioned
	MentionCompetitor bool `gorm:"default:false" json:"mention_competitor"`
	// Recommended: answer treats brand as preferred / recommended
	Recommended   bool   `gorm:"default:false" json:"recommended"`
	HasLink       bool   `gorm:"default:false" json:"has_link"`
	MatchedBrands string `gorm:"type:text" json:"matched_brands,omitempty"` // comma names
	Engine        string `gorm:"size:32" json:"engine,omitempty"`
	// Sentiment towards own brand: positive | neutral | negative | ""（未分析）
	Sentiment string `gorm:"size:16" json:"sentiment,omitempty"`
	// ListPosition: 品牌在回答推荐列表中的位次，0 = 不在列表或无列表
	ListPosition int `gorm:"default:0" json:"list_position"`
	// AnalyzeBy: llm | heuristic | skipped（fallback 假回答不分析）
	AnalyzeBy string `gorm:"size:16" json:"analyze_by,omitempty"`
	// SourcesJSON: 检索注入的来源 JSON（含 cited / own_domain 标志），DB 列，不直接出 API
	SourcesJSON string `gorm:"type:text" json:"-"`
	// Sources: SourcesJSON 的反序列化视图（store 读出后填充），不持久化
	Sources   []GeoSource `gorm:"-" json:"sources,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

func (GeoResult) TableName() string { return "vis_geo_results" }

// RankSnapshot is one engine's rank observation for a keyword (history for trends).
// Error 非空表示该引擎本次采集失败（Rank 无意义）；Rank=0 且 Error 为空表示采集成功但前 N 未收录。
type RankSnapshot struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	KeywordID uint64 `gorm:"not null;index:idx_snap_kw" json:"keyword_id"`
	// Engine: bing | baidu | google | sogou | so360 | duckduckgo | github；
	// "all" 仅用于错误标记行（如未填目标网页），其 Rank 无意义
	Engine    string    `gorm:"size:16;not null;index:idx_snap_kw" json:"engine"`
	Rank      int       `gorm:"default:0" json:"rank"`
	SerpURL   string    `gorm:"size:1024" json:"serp_url,omitempty"`
	Error     string    `gorm:"size:512" json:"error,omitempty"`
	TaskID    uint64    `gorm:"index" json:"task_id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (RankSnapshot) TableName() string { return "vis_rank_snapshots" }

// SerpTask is one batch SERP rank-check execution (manual or cron).
type SerpTask struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// Trigger: manual | cron
	Trigger string `gorm:"size:16;not null;default:manual" json:"trigger"`
	// KeywordID: 0 = 全部 tracking 关键词
	KeywordID uint64 `gorm:"default:0" json:"keyword_id"`
	// Status: running | done | failed
	Status     string     `gorm:"size:16;not null;default:running;index" json:"status"`
	Total      int        `gorm:"default:0" json:"total"`
	DoneCount  int        `gorm:"default:0" json:"done_count"`
	OKCount    int        `gorm:"default:0" json:"ok_count"`
	FailCount  int        `gorm:"default:0" json:"fail_count"`
	Error      string     `gorm:"size:512" json:"error,omitempty"`
	CreatedBy  uint64     `gorm:"default:0" json:"created_by"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (SerpTask) TableName() string { return "vis_serp_tasks" }
