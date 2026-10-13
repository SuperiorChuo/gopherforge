package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/storage"
	"github.com/google/uuid"
)

// 附件上传（M2.1 → M5 存储）：访客与坐席共用同一
// 端点；对象写入 Server.Storage（栈中为 MinIO，开发环境为本地磁盘），
// key 形如 "<yyyymm>/<uuid><ext>"，并经如下路径回放：
// /im/uploads/<key>.
//
// 访问模型：能力 URL。key 内嵌来自 google/uuid 的 v4 UUID，
// 其熵取自 crypto/rand（122 位，不可枚举），因此
// 持有 URL 即视为持有凭据。带过期时间的签名 URL 会破坏
// 消息历史：客户端在发送时会把 URL 持久化进消息内容 JSON，
// 历史消息按原样重放，因此任何内嵌的过期时间都会失效。
// 响应被标记为 Cache-Control: private，以免共享缓存
// 把一个访客的附件提供给另一个访客。

const (
	attachmentMaxBytes = 10 << 20 // 10 MB
)

// imageExts 中的扩展名在客户端内联渲染；fileExts 中的其余扩展名渲染为
// 下载链接。两个集合之外的扩展名一律拒绝。
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

var fileExts = map[string]bool{
	".pdf": true, ".txt": true, ".zip": true, ".doc": true, ".docx": true,
	".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".csv": true,
	".mp4": true, ".mp3": true,
}

// UploadAttachment 处理 POST /api/v1/im/attachments（multipart "file"）。
// 认证：访客 token 或坐席 token，解析顺序与 SendMessage 相同。
func (s *Server) UploadAttachment(c *gin.Context) {
	senderOK := false
	if tok := bearer(c); tok != "" {
		if _, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			senderOK = true
		}
	}
	if !senderOK {
		if _, ok := s.requireAgent(c); !ok {
			return
		}
	}

	fh, err := c.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, "missing file field")
		return
	}
	if fh.Size <= 0 || fh.Size > attachmentMaxBytes {
		Fail(c, http.StatusBadRequest, fmt.Sprintf("file size must be 1B-%dMB", attachmentMaxBytes>>20))
		return
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	msgType := ""
	switch {
	case imageExts[ext]:
		msgType = "image"
	case fileExts[ext]:
		msgType = "file"
	default:
		Fail(c, http.StatusBadRequest, "unsupported file type: "+ext)
		return
	}

	key := time.Now().Format("200601") + "/" + uuid.NewString() + ext
	src, err := fh.Open()
	if err != nil {
		Fail(c, http.StatusBadRequest, "unreadable file")
		return
	}
	defer src.Close()
	if err := s.Storage.Save(c.Request.Context(), key, src, fh.Size, fh.Header.Get("Content-Type")); err != nil {
		Fail(c, http.StatusInternalServerError, "storage unavailable")
		return
	}

	OK(c, gin.H{
		"url":      "/im/uploads/" + key,
		"name":     fh.Filename,
		"size":     fh.Size,
		"mime":     fh.Header.Get("Content-Type"),
		"msg_type": msgType,
	})
}

// ServeAttachment 处理 GET /im/uploads/*key：先从 Storage 流式读取，
// 失败时回退到遗留本地目录（im_uploads 卷上 MinIO 之前的文件）。
// 随机 UUID key 即访问能力（参见包注释），因此响应
// 必须避开共享缓存。
func (s *Server) ServeAttachment(c *gin.Context) {
	key, err := storage.CleanKey(c.Param("key"))
	if err != nil {
		Fail(c, http.StatusBadRequest, "bad path")
		return
	}
	body, size, contentType, err := s.Storage.Open(c.Request.Context(), key)
	if err != nil && s.UploadDir != "" && s.Storage.Type() != "local" {
		// 遗留回退：对象存储启用前上传的文件
		p := filepath.Join(s.UploadDir, filepath.FromSlash(key))
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			setAttachmentHeaders(c)
			c.File(p)
			return
		}
	}
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			Fail(c, http.StatusNotFound, "not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "storage unavailable")
		return
	}
	defer body.Close()
	setAttachmentHeaders(c)
	c.DataFromReader(http.StatusOK, size, contentType, body, nil)
}

// setAttachmentHeaders 将附件响应标记为共享代理不可缓存
// （URL 本身就是凭据），同时仍允许接收方的浏览器缓存
// 这份不可变对象。
func setAttachmentHeaders(c *gin.Context) {
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
}
