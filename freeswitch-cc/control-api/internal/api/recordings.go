package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// 录音访问：目录由 FS 录音卷只读挂载。文件名白名单校验 + filepath.Base，
// 杜绝路径穿越；播放/下载直接流式回源。

var recFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.wav$`)

type recordingView struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

// listRecordings handles GET /v1/recordings — 按修改时间倒序，最多 200 条。
func (s *Server) listRecordings(c *gin.Context) {
	entries, err := os.ReadDir(s.Cfg.RecDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"list": []recordingView{}, "total": 0})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	list := make([]recordingView, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".wav") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		list = append(list, recordingView{
			Name:       ent.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ModifiedAt > list[j].ModifiedAt })
	if len(list) > 200 {
		list = list[:200]
	}
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

// getRecording handles GET /v1/recordings/:file — 流式返回 wav。
func (s *Server) getRecording(c *gin.Context) {
	name := filepath.Base(c.Param("file"))
	if !recFileRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	path := filepath.Join(s.Cfg.RecDir, name)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	c.Header("Content-Type", "audio/wav")
	c.Header("Content-Disposition", `inline; filename="`+name+`"`)
	c.File(path)
}
