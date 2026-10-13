// Package publisher 定义社媒渠道发布的统一抽象。每个平台（微信/微博/小红书…）实现
// Publisher 接口作为一个 adapter，注册到 Registry。内容中台按 channel 分发到对应
// adapter，不感知平台细节。
//
// 两类能力并列：Publish（有官方 API 的平台直发）与 ExportPackage（无 API 的平台
// 半自动导出，AI 生成文案+素材，人工去 App 粘贴发）。adapter 按实际能力声明
// Capabilities，只实现自己支持的那类；不支持的返回对应 sentinel error。
package publisher

import (
	"context"
	"errors"

	"github.com/go-admin-kit/services/mp/internal/content"
)

// Channel 渠道标识（与 content 里的常量一致）。
type Channel = string

// Capabilities 声明一个渠道 adapter 的能力，供前端决定展示"发布"还是"导出"。
type Capabilities struct {
	Publish     bool     `json:"publish"`     // 支持 API 直发
	Export      bool     `json:"export"`      // 支持一键导出（半自动）
	MediaKinds  []string `json:"media_kinds"` // image / video
	MaxTitleLen int      `json:"max_title_len"`
	MaxBodyLen  int      `json:"max_body_len"`
	Note        string   `json:"note,omitempty"` // 如"小红书无官方发布 API，仅支持导出"
}

// PublishResult API 直发的结果。Async=true 表示发布是异步的（如微信 freepublish），
// 需后续 QueryStatus 或回调确认最终状态；此时 ExternalID 是异步任务 id。
type PublishResult struct {
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
	Async       bool   `json:"async"`
}

// PublishStatus 异步发布状态查询结果。
type PublishStatus struct {
	Done        bool   `json:"done"`
	Success     bool   `json:"success"`
	ExternalURL string `json:"external_url"`
	Error       string `json:"error,omitempty"`
}

// DownloadItem 导出包里的一个可下载素材。
type DownloadItem struct {
	Kind string `json:"kind"` // image | video
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// ExportPackage 半自动导出：适配目标平台格式的文案 + 可下载素材 + 操作提示。
// 前端展示"复制文案 / 下载素材"，用户去对应 App 手动发布。
type ExportPackage struct {
	PlatformText string         `json:"platform_text"` // 已按平台格式化的文案（含话题标签）
	Images       []DownloadItem `json:"images,omitempty"`
	Videos       []DownloadItem `json:"videos,omitempty"`
	Tips         string         `json:"tips,omitempty"`
}

// Publisher 是一个渠道 adapter。account 为 content.ChannelAccount（含 RefID 指向
// 渠道自有配置，或 ConfigJSON 平台专属配置）。
type Publisher interface {
	Channel() Channel
	Capabilities() Capabilities
	// Publish API 直发。不支持返回 ErrPublishUnsupported。
	Publish(ctx context.Context, account *content.ChannelAccount, post *content.Post) (PublishResult, error)
	// ExportPackage 半自动导出。不支持返回 ErrExportUnsupported。
	ExportPackage(ctx context.Context, account *content.ChannelAccount, post *content.Post) (ExportPackage, error)
	// QueryStatus 查异步发布结果（同步/不支持的可返回 done+success 直接完成）。
	QueryStatus(ctx context.Context, account *content.ChannelAccount, pub *content.PostPublication) (PublishStatus, error)
}

var (
	// ErrPublishUnsupported 该渠道无 API 直发（如小红书/知乎），应走 ExportPackage。
	ErrPublishUnsupported = errors.New("publisher: channel does not support API publish")
	// ErrExportUnsupported 该渠道不提供导出（少见；一般都支持）。
	ErrExportUnsupported = errors.New("publisher: channel does not support export")
	// ErrChannelNotRegistered Registry 里没有该渠道 adapter。
	ErrChannelNotRegistered = errors.New("publisher: channel not registered")
)

// Registry 按 channel 分发到 adapter。启动时各 adapter Register 进来。
type Registry struct {
	m map[Channel]Publisher
}

func NewRegistry() *Registry { return &Registry{m: map[Channel]Publisher{}} }

func (r *Registry) Register(p Publisher) { r.m[p.Channel()] = p }

func (r *Registry) Get(channel Channel) (Publisher, error) {
	p, ok := r.m[channel]
	if !ok {
		return nil, ErrChannelNotRegistered
	}
	return p, nil
}

// List 返回所有已注册渠道及其能力（供前端"发布中心"渲染渠道选择）。
func (r *Registry) List() map[Channel]Capabilities {
	out := make(map[Channel]Capabilities, len(r.m))
	for ch, p := range r.m {
		out[ch] = p.Capabilities()
	}
	return out
}
