package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// 软电话/WebPhone 直拨不走 cc originate，CHANNEL_CREATE 时向中台风控询问。
// 点击外呼已设 cc_risk_checked=1，语音通知已设 cc_announce=1，均跳过以免重复计数。
// HTTP 超时或 5xx：放行（媒体面不能因中台抖动全拒呼）；显式 403：uuid_kill。

var (
	guardUUIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
	localExtRe  = regexp.MustCompile(`^[0-9]{2,6}$`)
)

func (s *Server) GuardOutbound(ev map[string]string) {
	if s == nil || strings.TrimSpace(s.Cfg.RiskURL) == "" {
		return
	}
	if ev["Event-Name"] != "CHANNEL_CREATE" {
		return
	}
	uuid := ev["Unique-ID"]
	if uuid == "" || uuid != ev["Channel-Call-UUID"] || !guardUUIDRe.MatchString(uuid) {
		return
	}
	dir := ev["Call-Direction"]
	if dir == "" {
		dir = ev["Caller-Direction"]
	}
	if strings.EqualFold(dir, "inbound") {
		return
	}
	if ev["variable_cc_risk_checked"] == "1" || ev["variable_cc_announce"] == "1" {
		return
	}
	to := strings.TrimSpace(ev["Caller-Destination-Number"])
	if to == "" || localExtRe.MatchString(to) {
		return
	}
	from := strings.TrimSpace(ev["Caller-Username"])
	if from == "" {
		from = strings.TrimSpace(ev["Caller-Caller-ID-Number"])
	}
	go s.enforceSoftphoneRisk(uuid, from, to)
}

func (s *Server) enforceSoftphoneRisk(uuid, from, to string) {
	allow, reason := s.queryCCRisk(from, to)
	if allow {
		return
	}
	log.Printf("outbound guard: reject uuid=%s from=%s to=%s reason=%s", uuid, from, to, reason)
	if s.killChannel != nil {
		_ = s.killChannel(uuid)
		return
	}
	if s.ESL != nil {
		_, _ = s.ESL.API("uuid_kill " + uuid + " CALL_REJECTED")
	}
}

func (s *Server) queryCCRisk(from, to string) (allow bool, reason string) {
	body, err := json.Marshal(map[string]string{"from_ext": from, "to": to})
	if err != nil {
		return true, ""
	}
	client := s.riskHTTP
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	req, err := http.NewRequest(http.MethodPost, s.Cfg.RiskURL, bytes.NewReader(body))
	if err != nil {
		return true, ""
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Cfg.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(s.Cfg.WebhookSecret))
		_, _ = mac.Write(body)
		req.Header.Set("X-CC-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("outbound guard: risk-check error: %v", err)
		return true, ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		msg := "forbidden"
		var env struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &env) == nil && env.Message != "" {
			msg = env.Message
		}
		return false, msg
	}
	if resp.StatusCode >= 300 {
		log.Printf("outbound guard: risk-check status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		return true, ""
	}
	return true, ""
}
