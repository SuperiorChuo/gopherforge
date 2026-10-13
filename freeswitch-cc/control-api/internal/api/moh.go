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

// 排队/保持音乐管理：上传的音频落 MOH 卷，local_stream://moh 扫描该目录循环
// 播放；队列 moh-sound 也可直指单个文件。上传后 reload 让流重新扫描。

var mohFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.(wav|mp3)$`)

// listMOH handles GET /v1/moh.
func (s *Server) listMOH(c *gin.Context) {
	entries, err := os.ReadDir(s.Cfg.MOHDir)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type view struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
		// Path FS 侧路径，可直接填进队列 moh_sound
		Path string `json:"path"`
	}
	list := []view{}
	for _, ent := range entries {
		if ent.IsDir() || !mohFileRe.MatchString(ent.Name()) {
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
			Path:       "/etc/freeswitch/moh/" + ent.Name(),
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

// uploadMOH handles POST /v1/moh (multipart field "file").
func (s *Server) uploadMOH(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart field 'file' required"})
		return
	}
	name := filepath.Base(fh.Filename)
	if !mohFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file must be .wav or .mp3 (safe filename)"})
		return
	}
	if fh.Size > 50<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 50MB)"})
		return
	}
	if err := os.MkdirAll(s.Cfg.MOHDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(s.Cfg.MOHDir, name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// local_stream 重扫目录（moh 流正在放也会平滑切换）
	reloaded := true
	if _, err := s.ESL.API("local_stream stop moh"); err != nil {
		reloaded = false
	}
	if _, err := s.ESL.API("local_stream start moh"); err != nil {
		reloaded = false
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name, "path": "/etc/freeswitch/moh/" + name, "reloaded": reloaded})
}

// getMOH handles GET /v1/moh/:file — 试听。
func (s *Server) getMOH(c *gin.Context) {
	name := filepath.Base(c.Param("file"))
	if !mohFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	path := filepath.Join(s.Cfg.MOHDir, name)
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

// deleteMOH handles DELETE /v1/moh/:file.
func (s *Server) deleteMOH(c *gin.Context) {
	name := filepath.Base(c.Param("file"))
	if !mohFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	if err := os.Remove(filepath.Join(s.Cfg.MOHDir, name)); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = s.ESL.API("local_stream stop moh")
	_, _ = s.ESL.API("local_stream start moh")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
