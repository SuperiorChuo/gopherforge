package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// mod_callcenter 队列管理：DB → queues.xml 物化 + reload 生效；
// 坐席签入签出走 callcenter_config 运行时 API（即时），DB 侧 status 只是初始值。

var ccStrategies = map[string]bool{
	"ring-all": true, "longest-idle-agent": true, "round-robin": true,
	"top-down": true, "agent-with-least-talk-time": true, "agent-with-fewest-calls": true,
	"sequentially-by-agent-order": true, "ring-progressively": true,
}

var ccQueueNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// reloadCallcenter 让 mod_callcenter 重读配置（新队列/坐席结构变化时）。
func (s *Server) reloadCallcenter() bool {
	if _, err := s.ESL.API("reloadxml"); err != nil {
		return false
	}
	out, err := s.ESL.API("reload mod_callcenter")
	return err == nil && !strings.Contains(out, "-ERR")
}

func (s *Server) syncCCQueues() error {
	queues, err := s.Store.ListCCQueues()
	if err != nil {
		return err
	}
	agents, err := s.Store.ListQueueAgents("")
	if err != nil {
		return err
	}
	return extdir.SyncCCQueues(s.Cfg.CCQueueDir, s.Cfg.DialplanDir, queues, agents)
}

// parsePipeTable 解析 callcenter_config 的 |-分隔表格输出为 map 列表。
func parsePipeTable(raw string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return nil
	}
	header := strings.Split(strings.TrimRight(lines[0], "\r"), "|")
	var rows []map[string]string
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "+OK") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) != len(header) {
			continue
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			row[strings.TrimSpace(h)] = strings.TrimSpace(cols[i])
		}
		rows = append(rows, row)
	}
	return rows
}

type ccQueueReq struct {
	Name            string `json:"name" binding:"required"`
	AccessNumber    string `json:"access_number" binding:"required"`
	Strategy        string `json:"strategy"`
	MOHSound        string `json:"moh_sound"`
	MaxWaitTime     int    `json:"max_wait_time"`
	TimeoutNumber   string `json:"timeout_number"`
	TierRulesApply  bool   `json:"tier_rules_apply"`
	TierRuleWaitSec int    `json:"tier_rule_wait_sec"`
	// 深水区参数
	AnnounceSound               string `json:"announce_sound"`
	AnnounceFrequency           int    `json:"announce_frequency"`
	RecordTemplate              bool   `json:"record_template"`
	AbandonedResumeAllowed      bool   `json:"abandoned_resume_allowed"`
	RingProgressivelyDelay      int    `json:"ring_progressively_delay"`
	SkipAgentsWithExternalCalls bool   `json:"skip_agents_with_external_calls"`
	AgentNoAnswerStatus         string `json:"agent_no_answer_status"`
}

var ccAgentNoAnswerStatuses = map[string]bool{
	"": true, "On Break": true, "Logged Out": true,
}

func (r *ccQueueReq) validate(c *gin.Context) *store.CCQueue {
	bad := func(msg string) *store.CCQueue {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return nil
	}
	switch {
	case !ccQueueNameRe.MatchString(r.Name):
		return bad("name must be 2-32 lowercase letters/digits/_-")
	case !numberRe.MatchString(r.AccessNumber) || reservedExt(r.AccessNumber):
		return bad("invalid or reserved access_number")
	case r.Strategy != "" && !ccStrategies[r.Strategy]:
		return bad("unknown strategy")
	case r.TimeoutNumber != "" && !numberRe.MatchString(r.TimeoutNumber):
		return bad("invalid timeout_number")
	case r.MaxWaitTime < 0 || r.MaxWaitTime > 7200:
		return bad("max_wait_time must be 0-7200")
	case r.MOHSound != "" && !ivrGreetingRe.MatchString(r.MOHSound):
		return bad("invalid moh_sound")
	case r.AnnounceSound != "" && !ivrGreetingRe.MatchString(r.AnnounceSound):
		return bad("invalid announce_sound")
	case r.AnnounceFrequency < 0 || r.AnnounceFrequency > 300:
		return bad("announce_frequency must be 0-300")
	case r.RingProgressivelyDelay < 0 || r.RingProgressivelyDelay > 300:
		return bad("ring_progressively_delay must be 0-300")
	case !ccAgentNoAnswerStatuses[r.AgentNoAnswerStatus]:
		return bad("agent_no_answer_status must be empty/On Break/Logged Out")
	}
	return &store.CCQueue{
		Name: r.Name, AccessNumber: r.AccessNumber, Strategy: r.Strategy,
		MOHSound: r.MOHSound, MaxWaitTime: r.MaxWaitTime, TimeoutNumber: r.TimeoutNumber,
		TierRulesApply: r.TierRulesApply, TierRuleWaitSec: r.TierRuleWaitSec,
		AnnounceSound: r.AnnounceSound, AnnounceFrequency: r.AnnounceFrequency,
		RecordTemplate: r.RecordTemplate, AbandonedResumeAllowed: r.AbandonedResumeAllowed,
		RingProgressivelyDelay:      r.RingProgressivelyDelay,
		SkipAgentsWithExternalCalls: r.SkipAgentsWithExternalCalls,
		AgentNoAnswerStatus:         r.AgentNoAnswerStatus,
	}
}

// listCCQueues handles GET /v1/cc-queues — DB 配置 + 运行时排队人数/坐席状态。
func (s *Server) listCCQueues(c *gin.Context) {
	queues, err := s.Store.ListCCQueues()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	agents, err := s.Store.ListQueueAgents("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	agentsByQueue := map[string][]store.CCQueueAgent{}
	for _, a := range agents {
		agentsByQueue[a.QueueName] = append(agentsByQueue[a.QueueName], a)
	}
	// 运行时坐席状态（一次拉全量再按名分发）
	liveAgent := map[string]map[string]string{}
	if raw, err := s.ESL.API("callcenter_config agent list"); err == nil {
		for _, row := range parsePipeTable(raw) {
			liveAgent[row["name"]] = row
		}
	}
	type agentView struct {
		store.CCQueueAgent
		Status string `json:"status"` // Available/On Break/Logged Out
		State  string `json:"state"`  // Waiting/Receiving/In a queue call
	}
	// memberView 每个排队者的实时快照：号码 + 已等待秒 + 排位
	type memberView struct {
		CIDNumber string `json:"cid_number"`
		WaitSec   int64  `json:"wait_sec"`
		Position  int    `json:"position"`
	}
	type view struct {
		store.CCQueue
		Members       int          `json:"members"` // 当前排队人数
		MembersDetail []memberView `json:"members_detail"`
		Agents        []agentView  `json:"agents"`
	}
	now := time.Now().Unix()
	out := make([]view, 0, len(queues))
	for _, q := range queues {
		v := view{CCQueue: q, Agents: []agentView{}, MembersDetail: []memberView{}}
		if raw, err := s.ESL.API(fmt.Sprintf("callcenter_config queue list members %s@default", q.Name)); err == nil {
			rows := parsePipeTable(raw)
			v.Members = len(rows)
			for _, row := range rows {
				m := memberView{CIDNumber: row["cid_number"]}
				if je := row["joined_epoch"]; je != "" {
					if joined, perr := strconv.ParseInt(je, 10, 64); perr == nil && joined > 0 {
						if diff := now - joined; diff >= 0 {
							m.WaitSec = diff
						}
					}
				}
				v.MembersDetail = append(v.MembersDetail, m)
			}
			// 按等待时长降序 = 排队先后（等得越久越靠前），赋 1..N 位次
			sort.SliceStable(v.MembersDetail, func(i, j int) bool {
				return v.MembersDetail[i].WaitSec > v.MembersDetail[j].WaitSec
			})
			for i := range v.MembersDetail {
				v.MembersDetail[i].Position = i + 1
			}
		}
		for _, a := range agentsByQueue[q.Name] {
			av := agentView{CCQueueAgent: a, Status: "Unknown"}
			if row, ok := liveAgent[a.Ext+"@default"]; ok {
				av.Status = row["status"]
				av.State = row["state"]
			}
			v.Agents = append(v.Agents, av)
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, gin.H{"list": out, "total": len(out)})
}

// createCCQueue handles POST /v1/cc-queues.
func (s *Server) createCCQueue(c *gin.Context) {
	var req ccQueueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and access_number required"})
		return
	}
	q := req.validate(c)
	if q == nil {
		return
	}
	if err := s.Store.CreateCCQueue(q); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "queue name or access_number exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncCCQueues(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadCallcenter()})
}

// updateCCQueue handles PUT /v1/cc-queues/:name.
func (s *Server) updateCCQueue(c *gin.Context) {
	name := c.Param("name")
	existing, err := s.Store.GetCCQueue(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req ccQueueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name
	q := req.validate(c)
	if q == nil {
		return
	}
	q.ID = existing.ID
	q.CreatedAt = existing.CreatedAt
	if err := s.Store.UpdateCCQueue(q); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncCCQueues(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadCallcenter()})
}

// deleteCCQueue handles DELETE /v1/cc-queues/:name.
func (s *Server) deleteCCQueue(c *gin.Context) {
	name := c.Param("name")
	if !ccQueueNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if err := s.Store.DeleteCCQueue(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncCCQueues(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadCallcenter()})
}

type queueAgentReq struct {
	Ext         string `json:"ext" binding:"required"`
	Level       int    `json:"level"`
	Position    int    `json:"position"`
	WrapUpTime  int    `json:"wrap_up_time"`
	MaxNoAnswer int    `json:"max_no_answer"`
	// 拒接/示忙/无应答冷却秒（0=用队列默认）
	RejectDelayTime   int `json:"reject_delay_time"`
	BusyDelayTime     int `json:"busy_delay_time"`
	NoAnswerDelayTime int `json:"no_answer_delay_time"`
}

// upsertQueueAgent handles PUT /v1/cc-queues/:name/agents — 挂坐席/调层级。
func (s *Server) upsertQueueAgent(c *gin.Context) {
	name := c.Param("name")
	if _, err := s.Store.GetCCQueue(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	var req queueAgentReq
	if err := c.ShouldBindJSON(&req); err != nil || !numberRe.MatchString(req.Ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid ext required"})
		return
	}
	a := &store.CCQueueAgent{
		QueueName: name, Ext: req.Ext,
		Level: req.Level, Position: req.Position,
		WrapUpTime: req.WrapUpTime, MaxNoAnswer: req.MaxNoAnswer,
		RejectDelayTime: req.RejectDelayTime, BusyDelayTime: req.BusyDelayTime,
		NoAnswerDelayTime: req.NoAnswerDelayTime,
	}
	if err := s.Store.UpsertQueueAgent(a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncCCQueues(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadCallcenter()})
}

// removeQueueAgent handles DELETE /v1/cc-queues/:name/agents/:ext.
func (s *Server) removeQueueAgent(c *gin.Context) {
	name, ext := c.Param("name"), c.Param("ext")
	if !numberRe.MatchString(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ext"})
		return
	}
	if err := s.Store.DeleteQueueAgent(name, ext); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not in queue"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncCCQueues(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadCallcenter()})
}

type agentStatusReq struct {
	Ext    string `json:"ext" binding:"required"`
	Status string `json:"status" binding:"required"` // available | on-break | logged-out
}

// setAgentStatus handles POST /v1/cc-agents/status — 签入/示忙/签出（运行时即时生效）。
func (s *Server) setAgentStatus(c *gin.Context) {
	var req agentStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || !numberRe.MatchString(req.Ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ext and status required"})
		return
	}
	statusMap := map[string]string{
		"available":  "Available",
		"on-break":   "On Break",
		"logged-out": "Logged Out",
	}
	fsStatus, ok := statusMap[req.Status]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be available/on-break/logged-out"})
		return
	}
	out, err := s.ESL.API(fmt.Sprintf("callcenter_config agent set status %s@default '%s'", req.Ext, fsStatus))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.Contains(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": fsStatus})
}
