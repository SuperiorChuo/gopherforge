package wxclient

import (
	"context"
	"strconv"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// M5 图文草稿箱与发布。封面缩略图走永久素材（material.go AddPermanentMaterial
// type=thumb）拿 thumb_media_id；正文图走 UploadArticleImage 换微信 URL。

// DraftArticle 单篇图文草稿（对应微信 draft/add 的 articles 元素）。
type DraftArticle struct {
	Title              string `json:"title"`
	Author             string `json:"author,omitempty"`
	Digest             string `json:"digest,omitempty"`
	Content            string `json:"content"`        // HTML 正文
	ThumbMediaID       string `json:"thumb_media_id"` // 封面永久素材 id
	NeedOpenComment    int    `json:"need_open_comment,omitempty"`
	OnlyFansCanComment int    `json:"only_fans_can_comment,omitempty"`
}

// AddDraft 新增草稿箱，返回草稿的 media_id。
func (c *Client) AddDraft(ctx context.Context, account *store.MpAccount, articles []DraftArticle) (string, error) {
	body := map[string]any{"articles": articles}
	var out struct {
		MediaID string `json:"media_id"`
	}
	if err := c.doJSON(ctx, account, "POST", "/cgi-bin/draft/add", body, &out); err != nil {
		return "", err
	}
	return out.MediaID, nil
}

// SubmitFreepublish 提交发布（草稿 media_id → publish_id）。发布是异步的，
// 结果走回调事件 PUBLISHJOBFINISH。
func (c *Client) SubmitFreepublish(ctx context.Context, account *store.MpAccount, draftMediaID string) (string, error) {
	body := map[string]any{"media_id": draftMediaID}
	var out struct {
		PublishID int64 `json:"publish_id"`
	}
	if err := c.doJSON(ctx, account, "POST", "/cgi-bin/freepublish/submit", body, &out); err != nil {
		return "", err
	}
	if out.PublishID == 0 {
		return "", nil
	}
	return strconv.FormatInt(out.PublishID, 10), nil
}

// GetFreepublishStatus 主动查发布状态（回调之外的兜底）。
// 返回 publish_status（0=成功,1=发布中,2/3/4=失败类）与首篇 article_url。
func (c *Client) GetFreepublishStatus(ctx context.Context, account *store.MpAccount, publishID string) (status int, articleURL string, err error) {
	body := map[string]any{"publish_id": publishID}
	var out struct {
		PublishStatus int `json:"publish_status"`
		ArticleDetail struct {
			Item []struct {
				ArticleURL string `json:"article_url"`
			} `json:"item"`
		} `json:"article_detail"`
	}
	if err := c.doJSON(ctx, account, "POST", "/cgi-bin/freepublish/get", body, &out); err != nil {
		return 0, "", err
	}
	url := ""
	if len(out.ArticleDetail.Item) > 0 {
		url = out.ArticleDetail.Item[0].ArticleURL
	}
	return out.PublishStatus, url, nil
}
