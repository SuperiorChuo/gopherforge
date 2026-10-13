package wxclient

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// 素材类型常量。微信 type 取值：image | voice | video | thumb。
const (
	MediaTypeImage = "image"
	MediaTypeVoice = "voice"
	MediaTypeVideo = "video"
	MediaTypeThumb = "thumb"
)

// wxMediaField 是素材上传 multipart 的文件字段名（微信固定为 "media"）。
const wxMediaField = "media"

// tempMediaResp 是临时素材上传的响应。
type tempMediaResp struct {
	Type      string `json:"type"`
	MediaID   string `json:"media_id"`
	CreatedAt int64  `json:"created_at"`
}

// UploadTempMedia 上传临时素材（3 天有效）。mediaType 取 image/voice/video/thumb。
// 用于被动回复图片消息等临时场景。
//
//	POST /cgi-bin/media/upload?type=image
func (c *Client) UploadTempMedia(ctx context.Context, acc *store.MpAccount, mediaType, filename string, r io.Reader) (string, error) {
	var out tempMediaResp
	path := "/cgi-bin/media/upload?type=" + mediaType
	if err := c.doUpload(ctx, acc, path, wxMediaField, filename, r, nil, &out); err != nil {
		return "", err
	}
	return out.MediaID, nil
}

// permanentMaterialResp 是永久素材上传的响应。图片/缩略图返回 url；type=thumb
// 返回的 media_id 即图文封面用的 thumb_media_id。
type permanentMaterialResp struct {
	MediaID string `json:"media_id"`
	URL     string `json:"url"`
}

// AddPermanentMaterial 上传永久素材（长期有效，占素材库配额）。mediaType 取
// image/voice/video/thumb；type=thumb 用于图文封面（返回的 media_id 作为
// thumb_media_id）。image/thumb 会额外返回 url。
//
//	POST /cgi-bin/material/add_material?type=image
func (c *Client) AddPermanentMaterial(ctx context.Context, acc *store.MpAccount, mediaType, filename string, r io.Reader) (mediaID, url string, err error) {
	var out permanentMaterialResp
	path := "/cgi-bin/material/add_material?type=" + mediaType
	if err := c.doUpload(ctx, acc, path, wxMediaField, filename, r, nil, &out); err != nil {
		return "", "", err
	}
	return out.MediaID, out.URL, nil
}

// articleImageResp 是图文正文图片上传的响应（只回 url）。
type articleImageResp struct {
	URL string `json:"url"`
}

// UploadArticleImage 上传图文消息正文里的图片，返回可直接用作 <img src> 的 url。
// 该接口不占用素材库配额，也不返回 media_id，专供图文正文内嵌图。
//
//	POST /cgi-bin/media/uploadimg
func (c *Client) UploadArticleImage(ctx context.Context, acc *store.MpAccount, filename string, r io.Reader) (string, error) {
	var out articleImageResp
	if err := c.doUpload(ctx, acc, "/cgi-bin/media/uploadimg", wxMediaField, filename, r, nil, &out); err != nil {
		return "", err
	}
	imageURL := strings.TrimSpace(out.URL)
	parsed, err := url.ParseRequestURI(imageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || !strings.EqualFold(parsed.Hostname(), "mmbiz.qpic.cn") || parsed.Port() != "" {
		return "", fmt.Errorf("wx uploadimg: invalid image url")
	}
	return imageURL, nil
}
