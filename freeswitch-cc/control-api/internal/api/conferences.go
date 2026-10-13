package api

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// mod_conference 会议室管理：DB → 40_conf-<name>.xml 拨号计划物化 + reloadxml；
// 实时态（在会人数/录音/成员静音）走 `conference xml_list` 解析合并；成员管控
// （静音/踢出/锁定/录音）走 `conference <name> <action>` ESL 即时生效。
// 动态会议室（拨 39xx 随拨随建）不入库，仅在实时列表里标注 dynamic:true。

var confNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// confNumberRe 接入号：3xxx 四位数字。39xx 段留给动态会议（下方单独拦截）。
var confNumberRe = regexp.MustCompile(`^3[0-9]{3}$`)

var confPINRe = regexp.MustCompile(`^[0-9]{4,8}$`)

// confActions 成员/会议管控白名单；member 类需带 member_id。
var confMemberActions = map[string]bool{
	"mute": true, "unmute": true, "deaf": true, "undeaf": true, "kick": true,
}
var confRoomActions = map[string]bool{
	"lock": true, "unlock": true, "record_start": true, "record_stop": true,
}

// ---- conference xml_list 解析（结构见 mod_conference.c conference_xlist）----

type confXMLList struct {
	Conferences []confXMLConference `xml:"conference"`
}

type confXMLConference struct {
	Name        string          `xml:"name,attr"`
	MemberCount int             `xml:"member-count,attr"`
	Recording   string          `xml:"recording,attr"`
	Locked      string          `xml:"locked,attr"`
	Dynamic     string          `xml:"dynamic,attr"`
	Members     []confXMLMember `xml:"members>member"`
}

type confXMLMember struct {
	Type           string `xml:"type,attr"`
	ID             string `xml:"id"`
	CallerIDName   string `xml:"caller_id_name"`
	CallerIDNumber string `xml:"caller_id_number"`
	// can_speak=false ⇒ 已静音
	CanSpeak string `xml:"flags>can_speak"`
}

// liveMember 前端消费的成员视图。
type liveMember struct {
	MemberID       string `json:"member_id"`
	CallerIDName   string `json:"caller_id_name"`
	CallerIDNumber string `json:"caller_id_number"`
	Muted          bool   `json:"muted"`
}

// liveRoom 单个会议室的实时态。
type liveRoom struct {
	MemberCount int          `json:"member_count"`
	Recording   bool         `json:"recording"`
	Locked      bool         `json:"locked"`
	Dynamic     bool         `json:"dynamic"`
	Members     []liveMember `json:"members"`
}

// fetchLiveConferences 拉一次 conference xml_list，按会议室名归并实时态。
func (s *Server) fetchLiveConferences() map[string]liveRoom {
	out := map[string]liveRoom{}
	raw, err := s.ESL.API("conference xml_list")
	if err != nil {
		return out
	}
	// ESL 可能带前缀（-ERR 等），截取到 <conferences 起点
	idx := strings.Index(raw, "<conferences")
	if idx < 0 {
		return out
	}
	var parsed confXMLList
	if err := xml.Unmarshal([]byte(raw[idx:]), &parsed); err != nil {
		return out
	}
	for _, c := range parsed.Conferences {
		room := liveRoom{
			MemberCount: c.MemberCount,
			Recording:   c.Recording == "true",
			Locked:      c.Locked == "true",
			Dynamic:     c.Dynamic == "true",
			Members:     []liveMember{},
		}
		for _, m := range c.Members {
			if m.Type != "caller" { // 跳过 recording_node 等虚拟成员
				continue
			}
			room.Members = append(room.Members, liveMember{
				MemberID:       m.ID,
				CallerIDName:   m.CallerIDName,
				CallerIDNumber: m.CallerIDNumber,
				Muted:          m.CanSpeak == "false",
			})
		}
		out[c.Name] = room
	}
	return out
}

func (s *Server) syncConferences() error {
	rooms, err := s.Store.ListConferences()
	if err != nil {
		return err
	}
	return extdir.SyncConferences(s.Cfg.DialplanDir, rooms)
}

type conferenceReq struct {
	Name        string `json:"name"`
	Number      string `json:"number"`
	PIN         string `json:"pin"`
	MaxMembers  int    `json:"max_members"`
	AutoRecord  bool   `json:"auto_record"`
	Description string `json:"description"`
}

// validate 校验并转 store 模型；非法时已写响应并返回 nil。
func (r *conferenceReq) validate(c *gin.Context) *store.CCConference {
	bad := func(msg string) *store.CCConference {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return nil
	}
	r.Name = strings.TrimSpace(r.Name)
	r.Number = strings.TrimSpace(r.Number)
	r.PIN = strings.TrimSpace(r.PIN)
	switch {
	case !confNameRe.MatchString(r.Name):
		return bad("name must be 2-32 lowercase letters/digits/_-")
	case !confNumberRe.MatchString(r.Number):
		return bad("number must be 3xxx (four digits)")
	case strings.HasPrefix(r.Number, "39"):
		return bad("number 39xx is reserved for dynamic conferences")
	case r.PIN != "" && !confPINRe.MatchString(r.PIN):
		return bad("pin must be 4-8 digits")
	case r.MaxMembers < 0 || r.MaxMembers > 1000:
		return bad("max_members must be 0-1000")
	case len(r.Description) > 256:
		return bad("description too long")
	}
	return &store.CCConference{
		Name: r.Name, Number: r.Number, PIN: r.PIN,
		MaxMembers: r.MaxMembers, AutoRecord: r.AutoRecord, Description: r.Description,
	}
}

// listConferences handles GET /v1/conferences — DB 配置 + 实时态合并。
// 动态会议室（dyn- 前缀，DB 无记录）也一并列出并标注 dynamic:true。
func (s *Server) listConferences(c *gin.Context) {
	rooms, err := s.Store.ListConferences()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	live := s.fetchLiveConferences()

	type view struct {
		store.CCConference
		Active      bool         `json:"active"`
		MemberCount int          `json:"member_count"`
		Recording   bool         `json:"recording"`
		Locked      bool         `json:"locked"`
		Dynamic     bool         `json:"dynamic"`
		Members     []liveMember `json:"members"`
	}
	out := make([]view, 0, len(rooms)+len(live))
	seen := map[string]bool{}
	for _, r := range rooms {
		v := view{CCConference: r, Members: []liveMember{}}
		if lr, ok := live[r.Name+"@default"]; ok {
			v.Active = true
			v.MemberCount = lr.MemberCount
			v.Recording = lr.Recording
			v.Locked = lr.Locked
			v.Members = lr.Members
		}
		seen[r.Name+"@default"] = true
		out = append(out, v)
	}
	// 实时列表里没入库的会议室 = 动态会议室（dyn-39xx@default）。
	for name, lr := range live {
		if seen[name] {
			continue
		}
		short := strings.TrimSuffix(name, "@default")
		v := view{
			CCConference: store.CCConference{Name: short},
			Active:       true,
			MemberCount:  lr.MemberCount,
			Recording:    lr.Recording,
			Locked:       lr.Locked,
			Dynamic:      true,
			Members:      lr.Members,
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, gin.H{"list": out, "total": len(out)})
}

// createConference handles POST /v1/conferences.
func (s *Server) createConference(c *gin.Context) {
	var req conferenceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	conf := req.validate(c)
	if conf == nil {
		return
	}
	if err := s.Store.CreateConference(conf); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "conference name or number exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncConferences(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// updateConference handles PUT /v1/conferences/:name — 改 PIN/录音/上限/接入号。
func (s *Server) updateConference(c *gin.Context) {
	name := c.Param("name")
	existing, err := s.Store.GetConference(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conference not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req conferenceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name // 名字不可改（拨号计划以名字为 profile 实例）
	conf := req.validate(c)
	if conf == nil {
		return
	}
	conf.ID = existing.ID
	conf.CreatedAt = existing.CreatedAt
	if err := s.Store.UpdateConference(conf); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "number already used by another conference"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncConferences(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// deleteConference handles DELETE /v1/conferences/:name — 进行中先踢干净再删。
func (s *Server) deleteConference(c *gin.Context) {
	name := c.Param("name")
	if !confNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	// 进行中的会议先 kick all，避免删了拨号计划但会议还挂着
	_, _ = s.ESL.API(fmt.Sprintf("conference %s@default kick all", name))
	if err := s.Store.DeleteConference(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conference not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncConferences(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

type confActionReq struct {
	Action   string `json:"action" binding:"required"`
	MemberID string `json:"member_id"`
}

var confMemberIDRe = regexp.MustCompile(`^[0-9]{1,10}$`)

// conferenceAction handles POST /v1/conferences/:name/action — 成员/会议管控。
// 会议室名允许动态会议（dyn- 前缀），故此处不套 confNameRe。
func (s *Server) conferenceAction(c *gin.Context) {
	name := c.Param("name")
	if !confActionNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	var req confActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action required"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	isMember := confMemberActions[action]
	if !isMember && !confRoomActions[action] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action not allowed"})
		return
	}
	if isMember && !confMemberIDRe.MatchString(req.MemberID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_id required for member actions"})
		return
	}

	var cmd string
	switch action {
	case "mute", "unmute", "deaf", "undeaf", "kick":
		cmd = fmt.Sprintf("conference %s@default %s %s", name, action, req.MemberID)
	case "lock":
		cmd = fmt.Sprintf("conference %s@default lock", name)
	case "unlock":
		cmd = fmt.Sprintf("conference %s@default unlock", name)
	case "record_start":
		file := fmt.Sprintf("$${recordings_dir}/conf-%s-manual-%s.wav", name, time.Now().Format("20060102-150405"))
		cmd = fmt.Sprintf("conference %s@default record %s", name, file)
	case "record_stop":
		cmd = fmt.Sprintf("conference %s@default norecord all", name)
	}

	out, err := s.ESL.API(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.Contains(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": strings.TrimSpace(out)})
}

// confActionNameRe 覆盖固定会议室（support）与动态会议室（dyn-3901）两种命名。
var confActionNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
