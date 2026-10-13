package api

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// 坐席通话操控（批次 G）：转移 / 保持 / 停泊 / DTMF。
// 与 callcontrol.go 同套防注入约定：入参先正则校验再拼 ESL 命令。

var (
	// transferTargetRe 转移/DTMF 之类的目标号码：数字与 *#+ 组合，2-32 位
	transferTargetRe = regexp.MustCompile(`^[0-9*#+]{2,32}$`)
	// valetSlotRe 停泊车位：6100-6199 段（vanilla 的 6000-6099 被动态 IVR 压掉）
	valetSlotRe = regexp.MustCompile(`^61[0-9]{2}$`)
	// dtmfDigitsRe DTMF 串：仅 0-9*#，1-32 位
	dtmfDigitsRe = regexp.MustCompile(`^[0-9*#]{1,32}$`)
)

type transferReq struct {
	CallID string `json:"call_id" binding:"required"`
	To     string `json:"to" binding:"required"`
	// Both 为 true 时双方一起转（-both），否则只转当前腿
	Both bool `json:"both"`
}

// transferCall handles POST /v1/calls/transfer — 把通话转到目标号码（走 default 拨号计划）。
func (s *Server) transferCall(c *gin.Context) {
	var req transferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id and to required"})
		return
	}
	if !uuidRe.MatchString(req.CallID) || !transferTargetRe.MatchString(req.To) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call_id or to"})
		return
	}
	var cmd string
	if req.Both {
		cmd = fmt.Sprintf("uuid_transfer %s -both %s XML default", req.CallID, req.To)
	} else {
		cmd = fmt.Sprintf("uuid_transfer %s %s XML default", req.CallID, req.To)
	}
	out, err := s.ESL.API(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reply": strings.TrimSpace(out)})
}

type holdReq struct {
	CallID string `json:"call_id" binding:"required"`
	// Action on=进入保持 / off=恢复 / toggle=切换
	Action string `json:"action" binding:"required"`
}

// holdCall handles POST /v1/calls/hold — 通话保持/恢复（uuid_hold）。
func (s *Server) holdCall(c *gin.Context) {
	var req holdReq
	if err := c.ShouldBindJSON(&req); err != nil || !uuidRe.MatchString(req.CallID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id and action required"})
		return
	}
	// on 不带子命令，off/toggle 带子命令
	var cmd string
	switch req.Action {
	case "on":
		cmd = fmt.Sprintf("uuid_hold %s", req.CallID)
	case "off":
		cmd = fmt.Sprintf("uuid_hold off %s", req.CallID)
	case "toggle":
		cmd = fmt.Sprintf("uuid_hold toggle %s", req.CallID)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be on/off/toggle"})
		return
	}
	out, err := s.ESL.API(cmd)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reply": strings.TrimSpace(out)})
}

type parkReq struct {
	CallID string `json:"call_id" binding:"required"`
	// Slot 停泊车位号 6100-6199
	Slot string `json:"slot" binding:"required"`
}

// parkCall handles POST /v1/calls/park — 把通话停泊到指定车位（转到 valet 拨号计划）。
func (s *Server) parkCall(c *gin.Context) {
	var req parkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id and slot required"})
		return
	}
	if !uuidRe.MatchString(req.CallID) || !valetSlotRe.MatchString(req.Slot) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call_id or slot"})
		return
	}
	out, err := s.ESL.API(fmt.Sprintf("uuid_transfer %s %s XML default", req.CallID, req.Slot))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "slot": req.Slot, "reply": strings.TrimSpace(out)})
}

// parkedSlot 一条停泊记录
type parkedSlot struct {
	Slot   string `json:"slot"`
	Caller string `json:"caller"`
}

// listParked handles GET /v1/parked — 列出 cclot 停泊车位里的通话。
func (s *Server) listParked(c *gin.Context) {
	out, err := s.ESL.API("valet_info cclot")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	list := parseParked(out)
	c.JSON(http.StatusOK, gin.H{"total": len(list), "list": list})
}

// parseParked 容错解析 valet_info 返回的 XML。FS 各版本的属性名不一致
// （id/ext、cid_num/caller_id_number 都见过），优先取 caller 相关，取不到留空。
func parseParked(raw string) []parkedSlot {
	list := []parkedSlot{}
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-ERR") {
		return list
	}
	var doc struct {
		Extensions []struct {
			ID             string `xml:"id,attr"`
			Ext            string `xml:"ext,attr"`
			UUID           string `xml:"uuid,attr"`
			CIDNum         string `xml:"cid_num,attr"`
			CallerIDNumber string `xml:"caller_id_number,attr"`
			CIDName        string `xml:"cid_name,attr"`
			CallerIDName   string `xml:"caller_id_name,attr"`
		} `xml:"extension"`
	}
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		return list
	}
	for _, e := range doc.Extensions {
		slot := firstNonEmpty(e.ID, e.Ext)
		if slot == "" {
			continue
		}
		caller := firstNonEmpty(e.CIDNum, e.CallerIDNumber, e.CIDName, e.CallerIDName)
		list = append(list, parkedSlot{Slot: slot, Caller: caller})
	}
	return list
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

type dtmfReq struct {
	CallID string `json:"call_id" binding:"required"`
	Digits string `json:"digits" binding:"required"`
}

// sendDTMF handles POST /v1/calls/dtmf — 向通话发送 DTMF 按键（uuid_send_dtmf）。
func (s *Server) sendDTMF(c *gin.Context) {
	var req dtmfReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id and digits required"})
		return
	}
	if !uuidRe.MatchString(req.CallID) || !dtmfDigitsRe.MatchString(req.Digits) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call_id or digits"})
		return
	}
	out, err := s.ESL.API(fmt.Sprintf("uuid_send_dtmf %s %s", req.CallID, req.Digits))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if strings.HasPrefix(out, "-ERR") {
		c.JSON(http.StatusBadGateway, gin.H{"error": strings.TrimSpace(out)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reply": strings.TrimSpace(out)})
}
