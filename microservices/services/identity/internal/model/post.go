package model

import "time"

// Post stores job position data assignable to users (ruoyi-style).
// 表名带 sys_ 前缀：裸 "posts" 与社媒内容中台的 posts（内容库）撞名。
type Post struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"not null;default:1;uniqueIndex:ux_sys_posts_tenant_code,priority:1;index" json:"tenant_id"`
	Code      string    `gorm:"size:64;not null;uniqueIndex:ux_sys_posts_tenant_code,priority:2" json:"code"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Status    int8      `gorm:"default:1" json:"status"`
	Remark    string    `gorm:"size:500" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Post) TableName() string { return "sys_posts" }

// UserPost links users to posts.
type UserPost struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;uniqueIndex:ux_sys_user_posts_user_post,priority:1" json:"user_id"`
	PostID uint `gorm:"not null;uniqueIndex:ux_sys_user_posts_user_post,priority:2;index" json:"post_id"`
}

func (UserPost) TableName() string { return "sys_user_posts" }
