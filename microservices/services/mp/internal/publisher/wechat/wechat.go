// Package wechat 是微信公众号的渠道 adapter，实现 publisher.Publisher 接口。
// 它把已验证的微信发布链路（封面永久素材 → 草稿箱 → freepublish）包在统一抽象后
// 面，复用 store（mp_accounts / 微信专属配置）与 wxclient（微信 API）。
//
// 与内容中台的对接：content.ChannelAccount.RefID 指向 mp_accounts.id，adapter 据此
// 取回微信账号配置。wxclient/wxcrypto/wxmsg 一行不改。
package wechat

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxarticle"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
)

// Adapter 微信渠道 adapter。依赖注入 store（取账号）与 wxclient（调微信）。
type Adapter struct {
	store *store.Store
	wx    *wxclient.Client
}

func New(st *store.Store, wx *wxclient.Client) *Adapter {
	return &Adapter{store: st, wx: wx}
}

func (a *Adapter) Channel() publisher.Channel { return content.ChannelWechat }

func (a *Adapter) Capabilities() publisher.Capabilities {
	return publisher.Capabilities{
		Publish:     true,
		Export:      true,
		MediaKinds:  []string{"image"},
		MaxTitleLen: 64,
		MaxBodyLen:  20000,
		Note:        "微信公众号：支持 API 直发草稿箱/群发；也可导出图文包。",
	}
}

// resolveAccount 从 ChannelAccount.RefID 取回微信 mp_accounts 记录。
func (a *Adapter) resolveAccount(account *content.ChannelAccount) (*store.MpAccount, error) {
	if account == nil || account.RefID == 0 {
		return nil, fmt.Errorf("wechat: channel account 未关联公众号（ref_id=0）")
	}
	return a.store.GetAccount(account.RefID, account.TenantID)
}

// Publish 复用已验证的发布编排：封面 → 永久缩略素材 → 草稿箱 → 按 AutoPublish
// 决定 freepublish。返回 PublishResult（Async=true 当提交了 freepublish）。
func (a *Adapter) Publish(ctx context.Context, account *content.ChannelAccount, post *content.Post) (publisher.PublishResult, error) {
	if a.wx == nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 微信客户端未配置")
	}
	acc, err := a.resolveAccount(account)
	if err != nil {
		return publisher.PublishResult{}, err
	}
	if acc.Status != "active" {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 公众号账号已停用")
	}
	if post.CoverURL == "" {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 请先设置封面")
	}

	// 先读取并校验所有源图片，再开始产生微信侧写入。
	imgBytes, ctype, err := wxarticle.FetchImage(ctx, post.CoverURL)
	if err != nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 读取封面失败: %w", err)
	}
	rewrittenHTML, err := wxarticle.RewriteInlineImages(ctx, a.wx, acc, post.BodyHTML)
	if err != nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 处理正文图片失败: %w", err)
	}

	// 封面 → 微信永久缩略素材。
	thumbID, _, err := a.wx.AddPermanentMaterial(ctx, acc, "thumb", "cover"+wxarticle.ExtensionForContentType(ctype), bytes.NewReader(imgBytes))
	if err != nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 封面上传微信失败: %w", err)
	}

	// 草稿箱只接收已经微信化的局部正文，平台无关源稿保持不变。
	draftID, err := a.wx.AddDraft(ctx, acc, []wxclient.DraftArticle{{
		Title: post.Title, Digest: post.Summary, Content: rewrittenHTML, ThumbMediaID: thumbID,
	}})
	if err != nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 草稿箱失败: %w", err)
	}

	// 自动发布？未开则停在草稿箱（ExternalID=草稿 media_id，非异步）。
	if !acc.AutoPublish {
		return publisher.PublishResult{ExternalID: draftID, Async: false}, nil
	}
	pubID, err := a.wx.SubmitFreepublish(ctx, acc, draftID)
	if err != nil {
		return publisher.PublishResult{}, fmt.Errorf("wechat: 已存草稿，发布提交失败: %w", err)
	}
	// freepublish 是异步，最终结果走 PUBLISHJOBFINISH 回调或 QueryStatus。
	return publisher.PublishResult{ExternalID: pubID, Async: true}, nil
}

// QueryStatus 主动查 freepublish 结果。ExternalID 为 publish_id。
func (a *Adapter) QueryStatus(ctx context.Context, account *content.ChannelAccount, pub *content.PostPublication) (publisher.PublishStatus, error) {
	if a.wx == nil {
		return publisher.PublishStatus{}, fmt.Errorf("wechat: 微信客户端未配置")
	}
	acc, err := a.resolveAccount(account)
	if err != nil {
		return publisher.PublishStatus{}, err
	}
	if pub.ExternalID == "" {
		return publisher.PublishStatus{}, fmt.Errorf("wechat: 无 publish_id")
	}
	status, url, err := a.wx.GetFreepublishStatus(ctx, acc, pub.ExternalID)
	if err != nil {
		return publisher.PublishStatus{}, err
	}
	// publish_status: 0=成功, 1=发布中, 其他=失败
	switch status {
	case 0:
		return publisher.PublishStatus{Done: true, Success: true, ExternalURL: url}, nil
	case 1:
		return publisher.PublishStatus{Done: false}, nil
	default:
		return publisher.PublishStatus{Done: true, Success: false, Error: "publish_status=" + strconv.Itoa(status)}, nil
	}
}

// ExportPackage 导出图文包（微信也支持，方便运营复制到别处）。文案取正文纯文本。
func (a *Adapter) ExportPackage(_ context.Context, _ *content.ChannelAccount, post *content.Post) (publisher.ExportPackage, error) {
	text := post.Title + "\n\n" + stripHTML(post.BodyHTML)
	pkg := publisher.ExportPackage{
		PlatformText: text,
		Tips:         "复制文案后可粘贴到公众号编辑器；封面与图片见下方素材。",
	}
	if post.CoverURL != "" {
		pkg.Images = append(pkg.Images, publisher.DownloadItem{Kind: "image", URL: post.CoverURL, Name: "封面"})
	}
	for _, m := range post.Media {
		if m.Kind == "image" {
			pkg.Images = append(pkg.Images, publisher.DownloadItem{Kind: "image", URL: m.URL, Name: m.Name})
		}
	}
	return pkg, nil
}

// stripHTML 极简去标签（导出文案用，非严格）。
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte('\n')
		case !inTag:
			b.WriteRune(r)
		}
	}
	// 压缩多余空行
	out := strings.ReplaceAll(b.String(), "\n\n\n", "\n\n")
	return strings.TrimSpace(out)
}
