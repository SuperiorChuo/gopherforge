package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 动态分机管理：DB 为事实源，XML 物化到与 FS 共享的目录卷，改完 reloadxml。
// 静态分机（镜像模板 1000-1003）只读展示，1000-1019 归 vanilla 模板所有，禁止动态占用。

var (
	extRe      = regexp.MustCompile(`^[0-9]{4,6}$`)
	extPassRe  = regexp.MustCompile(`^[A-Za-z0-9]{4,32}$`)
	extLabelRe = regexp.MustCompile(`^[\p{Han}A-Za-z0-9 _-]{0,32}$`)
	// vmPassRe 语音信箱密码：4-8 位数字（*98 输入用，纯数字更适合话机键盘）。
	vmPassRe = regexp.MustCompile(`^[0-9]{4,8}$`)
)

type extensionView struct {
	Ext      string `json:"ext"`
	Password string `json:"password,omitempty"`
	Label    string `json:"label"`
	Source   string `json:"source"` // static | dynamic
	// VMEnabled/VMPassword 语音信箱状态（静态分机恒为关闭）
	VMEnabled  bool   `json:"vm_enabled"`
	VMPassword string `json:"vm_password,omitempty"`
	// 411 目录拼音名（两者都空则不进目录）
	DirFirstName string `json:"dir_first_name"`
	DirLastName  string `json:"dir_last_name"`
}

// extDirNameRe 411 目录拼音名：0-32 位小写字母（如 zhang/san）。
var extDirNameRe = regexp.MustCompile(`^[a-z]{0,32}$`)

var staticExtensions = []extensionView{
	// 静态分机密码与镜像模板（conf/directory/default/*.xml）保持一致，
	// 2026-08-08 起从弱口令 1234 换成独立强随机密码；改密码必须两处同步。
	{Ext: "1000", Password: "mNDhmMVpzZ6p", Label: "Agent A", Source: "static"},
	{Ext: "1001", Password: "pdE4QKmAdQAJ", Label: "Agent B", Source: "static"},
	{Ext: "1002", Password: "GmDCK9GyegX8", Label: "Agent C", Source: "static"},
	{Ext: "1003", Password: "Ptkku8dbWc5w", Label: "Agent D", Source: "static"},
}

// listExtensions handles GET /v1/extensions — 静态 + 动态合并。
func (s *Server) listExtensions(c *gin.Context) {
	dyn, err := s.Store.ListExtensions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	list := make([]extensionView, 0, len(staticExtensions)+len(dyn))
	list = append(list, staticExtensions...)
	for _, e := range dyn {
		list = append(list, extensionView{
			Ext: e.Ext, Password: e.Password, Label: e.Label, Source: "dynamic",
			VMEnabled: e.VMEnabled, VMPassword: e.VMPassword,
			DirFirstName: e.DirFirstName, DirLastName: e.DirLastName,
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

type createExtensionReq struct {
	Ext      string `json:"ext" binding:"required"`
	Password string `json:"password" binding:"required"`
	Label    string `json:"label"`
	// VMEnabled 开启语音信箱（无人接/占线 25 秒转留言，*98 听取）
	VMEnabled bool `json:"vm_enabled"`
	// VMPassword 语音信箱密码（4-8 位数字），空=同分机号
	VMPassword string `json:"vm_password"`
	// 411 目录拼音名
	DirFirstName string `json:"dir_first_name"`
	DirLastName  string `json:"dir_last_name"`
}

func reservedExt(ext string) bool {
	if n, err := strconv.Atoi(ext); err == nil && n >= 1000 && n <= 1019 {
		return true // vanilla 模板自带，容器重建时会被模板覆盖
	}
	return ext == "5000" || ext == "9196" // 演示队列 / 回声测试
}

// createExtension handles POST /v1/extensions.
func (s *Server) createExtension(c *gin.Context) {
	var req createExtensionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ext and password required"})
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.VMPassword = strings.TrimSpace(req.VMPassword)
	req.DirFirstName = strings.TrimSpace(req.DirFirstName)
	req.DirLastName = strings.TrimSpace(req.DirLastName)
	switch {
	case !extRe.MatchString(req.Ext):
		c.JSON(http.StatusBadRequest, gin.H{"error": "ext must be 4-6 digits"})
		return
	case reservedExt(req.Ext):
		c.JSON(http.StatusBadRequest, gin.H{"error": "ext is reserved (1000-1019/5000/9196)"})
		return
	case !extPassRe.MatchString(req.Password):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 4-32 alphanumeric"})
		return
	case !extLabelRe.MatchString(req.Label):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid label"})
		return
	case req.VMPassword != "" && !vmPassRe.MatchString(req.VMPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": "voicemail password must be 4-8 digits"})
	case !extDirNameRe.MatchString(req.DirFirstName) || !extDirNameRe.MatchString(req.DirLastName):
		c.JSON(http.StatusBadRequest, gin.H{"error": "dir name must be 0-32 lowercase letters (pinyin)"})
		return
	}

	e := &store.Extension{
		Ext: req.Ext, Password: req.Password, Label: req.Label,
		VMEnabled: req.VMEnabled, VMPassword: req.VMPassword,
		DirFirstName: req.DirFirstName, DirLastName: req.DirLastName,
	}
	if err := s.Store.CreateExtension(e); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "ext already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.Write(s.Cfg.ExtDir, *e); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	reloaded := s.reloadXML()
	c.JSON(http.StatusOK, gin.H{"ok": true, "ext": e.Ext, "reloaded": reloaded})
}

// updateExtension handles PUT /v1/extensions/:ext（仅动态分机）。
// 密码必填（自定义或随机，无默认）——改密码即改目录 XML + reloadxml，
// 已注册话机会因密码不符被拒，属预期安全行为。
func (s *Server) updateExtension(c *gin.Context) {
	ext := c.Param("ext")
	if !extRe.MatchString(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext"})
		return
	}
	var req createExtensionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ext and password required"})
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.VMPassword = strings.TrimSpace(req.VMPassword)
	req.DirFirstName = strings.TrimSpace(req.DirFirstName)
	req.DirLastName = strings.TrimSpace(req.DirLastName)
	switch {
	case !extPassRe.MatchString(req.Password):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 4-32 alphanumeric"})
		return
	case !extLabelRe.MatchString(req.Label):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid label"})
		return
	case req.VMPassword != "" && !vmPassRe.MatchString(req.VMPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": "voicemail password must be 4-8 digits"})
		return
	case !extDirNameRe.MatchString(req.DirFirstName) || !extDirNameRe.MatchString(req.DirLastName):
		c.JSON(http.StatusBadRequest, gin.H{"error": "dir name must be 0-32 lowercase letters (pinyin)"})
		return
	}
	e := &store.Extension{
		Ext: req.Ext, Password: req.Password, Label: req.Label,
		VMEnabled: req.VMEnabled, VMPassword: req.VMPassword,
		DirFirstName: req.DirFirstName, DirLastName: req.DirLastName,
	}
	if err := s.Store.UpdateExtension(ext, e); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ext not found (static extensions cannot be edited)"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.Write(s.Cfg.ExtDir, *e); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	reloaded := s.reloadXML()
	c.JSON(http.StatusOK, gin.H{"ok": true, "ext": ext, "reloaded": reloaded})
}

// deleteExtension handles DELETE /v1/extensions/:ext（仅动态分机）。
func (s *Server) deleteExtension(c *gin.Context) {
	ext := c.Param("ext")
	if !extRe.MatchString(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext"})
		return
	}
	if err := s.Store.DeleteExtension(ext); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ext not found (static extensions cannot be deleted)"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.Remove(s.Cfg.ExtDir, ext); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove xml: " + err.Error()})
		return
	}
	reloaded := s.reloadXML()
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": reloaded})
}

// reloadXML 尽力而为：ESL 不通时分机文件已落盘，FS 起来后 seed/重载也能生效。
func (s *Server) reloadXML() bool {
	if _, err := s.ESL.API("reloadxml"); err != nil {
		return false
	}
	return true
}
