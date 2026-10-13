package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// 语音信箱留言访问：留言由 mod_voicemail 落到与 FS 共享的存储卷（只读挂 VMDir）。
// 目录结构约定 default/<domain>/<ext>/msg_<uuid>.wav，但真实层级 E2E 校准——
// 这里做容错扫描：递归找 msg*.wav，从路径里回推分机号（末级匹配 2-6 位数字的
// 目录段），文件 mtime 作留言时间。播放/删除按 <ext>/<file> 在卷内定位真实文件，
// filepath.Base + 白名单正则杜绝路径穿越。

var (
	// 留言文件名：mod_voicemail 录音默认 msg_<uuid>.wav；容错也接受其它 msg*.wav。
	vmMsgFileRe = regexp.MustCompile(`(?i)^msg.*\.wav$`)
	// :file 路由参数白名单（安全字符集 + .wav 后缀）
	vmFileParamRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.wav$`)
	// 分机号：2-6 位数字（目录段/查询参数校验）
	vmExtRe = regexp.MustCompile(`^[0-9]{2,6}$`)
	// 路径段里像分机号的目录（回推 ext 用）
	vmExtSegRe = regexp.MustCompile(`^[0-9]{2,6}$`)
)

type voicemailView struct {
	Ext        string `json:"ext"`
	File       string `json:"file"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

// extFromVMPath 从留言文件相对 VMDir 的路径回推分机号：取最靠近文件的、
// 形如 2-6 位数字的目录段（default/<domain>/<ext>/msg_x.wav → <ext>）。
func extFromVMPath(rel string) string {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	// 去掉文件名本身，从后往前找第一个纯数字段
	for i := len(segs) - 2; i >= 0; i-- {
		if vmExtSegRe.MatchString(segs[i]) {
			return segs[i]
		}
	}
	return ""
}

// listVoicemails handles GET /v1/voicemails?ext= — 递归扫描留言卷。
func (s *Server) listVoicemails(c *gin.Context) {
	extFilter := strings.TrimSpace(c.Query("ext"))
	if extFilter != "" && !vmExtRe.MatchString(extFilter) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext"})
		return
	}
	list := []voicemailView{}
	err := filepath.WalkDir(s.Cfg.VMDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// 单个坏目录跳过，不中断整体扫描
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !vmMsgFileRe.MatchString(d.Name()) {
			return nil
		}
		rel, rerr := filepath.Rel(s.Cfg.VMDir, path)
		if rerr != nil {
			return nil
		}
		ext := extFromVMPath(rel)
		if ext == "" {
			return nil
		}
		if extFilter != "" && ext != extFilter {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		list = append(list, voicemailView{
			Ext:        ext,
			File:       d.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 最新的留言排前面
	sort.Slice(list, func(i, j int) bool { return list[i].ModifiedAt > list[j].ModifiedAt })
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

// locateVoicemail 在留言卷里按 <ext>/<file> 定位真实文件（容错目录层级）：
// 递归找 base 名匹配、且路径含该 ext 目录段的第一个文件。ext/file 均已白名单校验，
// 比对 filepath.Base 杜绝路径穿越。
func (s *Server) locateVoicemail(ext, file string) (string, bool) {
	var found string
	_ = filepath.WalkDir(s.Cfg.VMDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() != file {
			return nil
		}
		rel, rerr := filepath.Rel(s.Cfg.VMDir, path)
		if rerr != nil {
			return nil
		}
		if extFromVMPath(rel) != ext {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	return found, found != ""
}

// getVoicemail handles GET /v1/voicemails/:ext/:file — 流式回放。
func (s *Server) getVoicemail(c *gin.Context) {
	ext := c.Param("ext")
	file := filepath.Base(c.Param("file"))
	if !vmExtRe.MatchString(ext) || !vmFileParamRe.MatchString(file) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext or file"})
		return
	}
	path, ok := s.locateVoicemail(ext, file)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "voicemail not found"})
		return
	}
	c.Header("Content-Type", "audio/wav")
	c.Header("Content-Disposition", `inline; filename="`+file+`"`)
	c.File(path)
}

// deleteVoicemail handles DELETE /v1/voicemails/:ext/:file — 删除留言文件。
// MWI（*98 灯/未读计数）刷新 E2E 校准（vm_read <ext>@<domain> 之类），先只删文件。
func (s *Server) deleteVoicemail(c *gin.Context) {
	ext := c.Param("ext")
	file := filepath.Base(c.Param("file"))
	if !vmExtRe.MatchString(ext) || !vmFileParamRe.MatchString(file) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext or file"})
		return
	}
	path, ok := s.locateVoicemail(ext, file)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "voicemail not found"})
		return
	}
	if err := os.Remove(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
