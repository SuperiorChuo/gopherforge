package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 班长话务监控台（批次 L）：把 mod_callcenter 的队列/坐席/成员运行时状态 +
// 活动通道一次聚合下发。等待时长在服务端算好（joined_epoch 与当前 epoch 差），
// 前端只渲染，不拿 epoch 自己算。

// monitorMember 排队成员：等待中的一路来电。
type monitorMember struct {
	CIDNumber string `json:"cid_number"`
	WaitSec   int    `json:"wait_sec"` // 已等待秒数（服务端算）
	Position  int    `json:"position"` // 排队位置（从 1 起）
	State     string `json:"state"`    // Waiting / Trying / Answered ...
}

// monitorQueue 单队列盯盘视图。
type monitorQueue struct {
	Name           string          `json:"name"`
	Strategy       string          `json:"strategy"`
	Members        []monitorMember `json:"members"`
	LongestWaitSec int             `json:"longest_wait_sec"`
	CallsAnswered  int             `json:"calls_answered"`
	CallsAbandoned int             `json:"calls_abandoned"`
}

// monitorAgent 坐席网格里的一格。
type monitorAgent struct {
	Ext    string `json:"ext"`    // name 去掉 @default
	Status string `json:"status"` // Available / On Break / Logged Out
	State  string `json:"state"`  // Waiting / Receiving / In a queue call
}

// monitorChannel 活动通话行（复用 show channels as json 的字段）。
type monitorChannel struct {
	UUID        string `json:"uuid"`
	Direction   string `json:"direction"`
	CIDNum      string `json:"cid_num"`
	Dest        string `json:"dest"`
	CallState   string `json:"callstate"`
	Application string `json:"application"`
	Created     string `json:"created"`
}

// ccMonitor handles GET /v1/cc-monitor — 队列 + 坐席 + 通道一次聚合。
func (s *Server) ccMonitor(c *gin.Context) {
	now := time.Now().Unix()
	c.JSON(http.StatusOK, gin.H{
		"queues":   s.monitorQueues(now),
		"agents":   s.monitorAgents(),
		"channels": s.monitorChannels(),
		"ts":       now,
	})
}

// monitorQueues 逐队列拉配置行 + 成员明细。
func (s *Server) monitorQueues(now int64) []monitorQueue {
	out := []monitorQueue{}
	raw, err := s.ESL.API("callcenter_config queue list")
	if err != nil {
		return out
	}
	for _, row := range parsePipeTable(raw) {
		name := strings.TrimSuffix(row["name"], "@default")
		if name == "" {
			continue
		}
		q := monitorQueue{
			Name:           name,
			Strategy:       row["strategy"],
			Members:        []monitorMember{},
			CallsAnswered:  atoiSafe(row["calls_answered"]),
			CallsAbandoned: atoiSafe(row["calls_abandoned"]),
		}
		q.Members, q.LongestWaitSec = s.monitorMembers(row["name"], now)
		out = append(out, q)
	}
	return out
}

// monitorMembers 拉单队列成员，服务端算等待秒 + 排队位置。
// FS 每行带 system_epoch（当前时刻），优先用它算差，缺失才退回进程 now。
func (s *Server) monitorMembers(queueName string, now int64) ([]monitorMember, int) {
	members := []monitorMember{}
	longest := 0
	// queue list members 期望全名（name@context），列表行里的 name 已含 @default
	raw, err := s.ESL.API(fmt.Sprintf("callcenter_config queue list members %s", queueName))
	if err != nil {
		return members, longest
	}
	rows := parsePipeTable(raw)
	// 只统计仍在排队/试呼的成员并按加入时间升序，越早加入位置越靠前
	waiting := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		st := row["state"]
		if st == "Waiting" || st == "Trying" {
			waiting = append(waiting, row)
		}
	}
	sort.SliceStable(waiting, func(i, j int) bool {
		return atoiSafe(waiting[i]["joined_epoch"]) < atoiSafe(waiting[j]["joined_epoch"])
	})
	for i, row := range waiting {
		joined := int64(atoiSafe(row["joined_epoch"]))
		ref := now
		if se := int64(atoiSafe(row["system_epoch"])); se > 0 {
			ref = se
		}
		wait := 0
		if joined > 0 && ref >= joined {
			wait = int(ref - joined)
		}
		if wait > longest {
			longest = wait
		}
		members = append(members, monitorMember{
			CIDNumber: row["cid_number"],
			WaitSec:   wait,
			Position:  i + 1,
			State:     row["state"],
		})
	}
	return members, longest
}

// monitorAgents 全量坐席状态（name 去 @default）。
func (s *Server) monitorAgents() []monitorAgent {
	out := []monitorAgent{}
	raw, err := s.ESL.API("callcenter_config agent list")
	if err != nil {
		return out
	}
	for _, row := range parsePipeTable(raw) {
		ext := strings.TrimSuffix(row["name"], "@default")
		if ext == "" {
			continue
		}
		out = append(out, monitorAgent{
			Ext:    ext,
			Status: row["status"],
			State:  row["state"],
		})
	}
	return out
}

// monitorChannels 复用 show channels as json，只取盯盘要用的字段。
func (s *Server) monitorChannels() []monitorChannel {
	out := []monitorChannel{}
	raw, err := s.ESL.API("show channels as json")
	if err != nil {
		return out
	}
	var parsed struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	for _, r := range parsed.Rows {
		out = append(out, monitorChannel{
			UUID:        rowStr(r, "uuid"),
			Direction:   rowStr(r, "direction"),
			CIDNum:      rowStr(r, "cid_num"),
			Dest:        rowStr(r, "dest"),
			CallState:   rowStr(r, "callstate"),
			Application: rowStr(r, "application"),
			Created:     rowStr(r, "created"),
		})
	}
	return out
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func rowStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
