package store

import "time"

// MpArticle AI 生成的图文草稿与发布状态机（M5）。
// 状态流转：draft_ai(AI生成) → drafted(已进微信草稿箱) → publishing(已提交发布)
//
//	→ published(发布成功) / failed(失败)。
type MpArticle struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index:idx_mp_articles_tenant_status,priority:1" json:"tenant_id"`
	AccountID uint64 `gorm:"not null;index:idx_mp_articles_tenant_account" json:"account_id"`

	Title       string `gorm:"size:128" json:"title"`
	Author      string `gorm:"size:64" json:"author"`
	Digest      string `gorm:"size:256" json:"digest"` // 摘要
	ContentHTML string `gorm:"type:text" json:"content_html"`
	// CoverURL 可能是 http URL 或 data:base64（AI 生图部分中转站只回 base64），
	// 用 text 容纳大 data URL。预览用；发布时转成微信永久素材。
	CoverURL string `gorm:"type:text" json:"cover_url"`

	// 微信侧标识（发布链路逐步回填）
	ThumbMediaID string `gorm:"size:128" json:"thumb_media_id"`   // 封面永久素材 id
	DraftMediaID string `gorm:"size:128" json:"draft_media_id"`   // 草稿箱 media_id
	PublishID    string `gorm:"size:128;index" json:"publish_id"` // freepublish publish_id
	ArticleURL   string `gorm:"size:1024" json:"article_url"`     // 发布后链接

	Status string `gorm:"size:32;not null;default:draft_ai;index:idx_mp_articles_tenant_status,priority:2" json:"status"`

	AIPrompt  string    `gorm:"type:text" json:"ai_prompt"` // 留档生成提示
	CreatedBy uint64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MpArticle) TableName() string { return "mp_articles" }
