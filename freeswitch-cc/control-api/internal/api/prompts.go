package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// 提示音库：上传/TTS 预生成的 wav/mp3 落 prompts 卷，队列 announce-sound、
// IVR greeting、拨号计划 playback 可直指 /etc/freeswitch/prompts/<name>。
// 与 MOH 不同：普通文件即取即播，无需 local_stream 流重启。

var promptFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.(wav|mp3)$`)

// listPrompts handles GET /v1/prompts.
func (s *Server) listPrompts(c *gin.Context) {
	entries, err := os.ReadDir(s.Cfg.PromptsDir)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type view struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
		// Path FS 侧路径，可直接填进队列 announce-sound / IVR greeting
		Path string `json:"path"`
	}
	list := []view{}
	for _, ent := range entries {
		if ent.IsDir() || !promptFileRe.MatchString(ent.Name()) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		list = append(list, view{
			Name:       ent.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
			Path:       "/etc/freeswitch/prompts/" + ent.Name(),
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

// uploadPrompt handles POST /v1/prompts (multipart field "file").
func (s *Server) uploadPrompt(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart field 'file' required"})
		return
	}
	name := filepath.Base(fh.Filename)
	if !promptFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file must be .wav or .mp3 (safe filename)"})
		return
	}
	if fh.Size > 50<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 50MB)"})
		return
	}
	if err := os.MkdirAll(s.Cfg.PromptsDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(s.Cfg.PromptsDir, name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 普通文件即取即播，无需重启流
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name, "path": "/etc/freeswitch/prompts/" + name})
}

// getPrompt handles GET /v1/prompts/:file — 试听。
func (s *Server) getPrompt(c *gin.Context) {
	name := filepath.Base(c.Param("file"))
	if !promptFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	path := filepath.Join(s.Cfg.PromptsDir, name)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ct := "audio/wav"
	if strings.HasSuffix(name, ".mp3") {
		ct = "audio/mpeg"
	}
	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", name))
	c.File(path)
}

// deletePrompt handles DELETE /v1/prompts/:file.
func (s *Server) deletePrompt(c *gin.Context) {
	name := filepath.Base(c.Param("file"))
	if !promptFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	if err := os.Remove(filepath.Join(s.Cfg.PromptsDir, name)); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
