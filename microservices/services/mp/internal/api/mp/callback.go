package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxcrypto"
	"github.com/go-admin-kit/services/mp/internal/wxmsg"
)

// CallbackVerify handles GET /api/v1/mp/callback/:appid — 微信"服务器配置"验签。
// 校验 sha1(sort(token,timestamp,nonce))==signature，通过则裸返回 echostr。
func (s *Server) CallbackVerify(c *gin.Context) {
	appID := c.Param("appid")
	acc, err := s.Store.GetAccountByAppID(appID)
	if err != nil {
		c.String(http.StatusNotFound, "unknown appid")
		return
	}
	signature := c.Query("signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	echostr := c.Query("echostr")
	if !wxcrypto.Verify(acc.Token, timestamp, nonce, signature) {
		c.String(http.StatusForbidden, "signature mismatch")
		return
	}
	// 微信要求裸文本 echostr，不能套 JSON 信封。
	c.String(http.StatusOK, echostr)
}

// CallbackMessage handles POST /api/v1/mp/callback/:appid — 收消息 + 被动回复。
// 支持明文/兼容模式与安全模式（encrypt_type=aes）。5 秒响应铁律：只做同步简单
// 回复，复杂逻辑（AI/客服）走异步下发。
func (s *Server) CallbackMessage(c *gin.Context) {
	appID := c.Param("appid")
	acc, err := s.Store.GetAccountByAppID(appID)
	if err != nil {
		c.String(http.StatusNotFound, "unknown appid")
		return
	}

	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "read body failed")
		return
	}

	encryptType := c.Query("encrypt_type")
	safeMode := encryptType == "aes" || acc.EncryptMode == "safe"

	var msgBody []byte                // 待解析的明文消息 XML
	var aesCipher *wxcrypto.AESCipher // 安全模式回复加密用；明文模式为 nil

	if safeMode {
		// 安全模式：body 是 <Encrypt> 密文，msg_signature 覆盖 Encrypt。
		env, perr := wxmsg.ParseEncrypted(raw)
		if perr != nil || env.Encrypt == "" {
			c.String(http.StatusOK, "success")
			return
		}
		msgSig := c.Query("msg_signature")
		if !wxcrypto.VerifyMsg(acc.Token, timestamp, nonce, env.Encrypt, msgSig) {
			c.String(http.StatusForbidden, "msg signature mismatch")
			return
		}
		aesKey := ""
		if s.AESKeyForAcct != nil {
			aesKey = s.AESKeyForAcct(acc)
		}
		if aesKey == "" {
			c.String(http.StatusInternalServerError, "aes key not configured")
			return
		}
		ac, cerr := wxcrypto.NewAESCipher(aesKey, acc.AppID)
		if cerr != nil {
			c.String(http.StatusInternalServerError, "aes key invalid")
			return
		}
		plain, derr := ac.Decrypt(env.Encrypt)
		if derr != nil {
			c.String(http.StatusOK, "success") // 解密失败不重推
			return
		}
		msgBody = plain
		aesCipher = ac
	} else {
		// 明文/兼容模式：GET 同款 signature 校验（防伪造）。
		if !wxcrypto.Verify(acc.Token, timestamp, nonce, c.Query("signature")) {
			c.String(http.StatusForbidden, "signature mismatch")
			return
		}
		msgBody = raw
	}

	in, err := wxmsg.Parse(msgBody)
	if err != nil {
		c.String(http.StatusOK, "success")
		return
	}
	isPublishResult := in.MsgType == "event" && in.Event == "PUBLISHJOBFINISH"
	if isPublishResult {
		publishID := strings.TrimSpace(in.PublishEventInfo.PublishID)
		publishStatus := strings.TrimSpace(in.PublishEventInfo.PublishStatus)
		if err := s.handlePublishResult(acc, publishID, publishStatus); err != nil {
			log.Printf("mp publish callback update failed: tenant_id=%d account_id=%d publish_id=%q: %v", acc.TenantID, acc.ID, publishID, err)
			c.String(http.StatusInternalServerError, "publish result update failed")
			return
		}
	}

	// Disabled accounts must not archive or reply to ordinary inbound traffic,
	// but a verified in-flight publish result still settles existing records.
	if acc.Status != "active" {
		c.String(http.StatusOK, "success")
		return
	}

	// Subscribe state must be updated before inbound-message deduplication: if
	// the database write fails and WeChat retries, the retry must not be skipped
	// merely because message archival succeeded on the first attempt.
	if in.MsgType == "event" && (in.Event == "subscribe" || in.Event == "unsubscribe") {
		eventAt := inboundMessageTime(in.CreateTime, time.Now())
		if err := s.Store.SetFanSubscribed(acc.TenantID, acc.ID, in.FromUserName, in.Event == "subscribe", eventAt); err != nil {
			c.String(http.StatusInternalServerError, "fan state update failed")
			return
		}
	}

	// 幂等：微信 5s 无响应会重推 3 次。发布事件没有 MsgId，以稳定摘要
	// 作为归档键；普通带 MsgId 的消息由数据库部分唯一索引原子去重。
	msgID := in.MsgIDString()
	if isPublishResult && msgID == "" {
		digest := sha256.Sum256([]byte(in.PublishEventInfo.PublishID + ":" + in.PublishEventInfo.PublishStatus))
		msgID = "publish:" + hex.EncodeToString(digest[:24])
	}

	// 留档入站消息（存解密后的明文 XML）。失败时让微信重试，避免管理台消息
	// 与 48 小时客服窗口的事实来源永久丢失。
	inserted, err := s.Store.ArchiveInboundMessage(&store.MpMessage{
		TenantID: acc.TenantID, AccountID: acc.ID,
		OpenID: in.FromUserName, Direction: "in",
		MsgType: in.MsgType, Content: in.Content, MsgID: msgID,
		Event: in.Event, RawBody: string(msgBody),
		CreatedAt: inboundMessageTime(in.CreateTime, time.Now()),
	})
	if err != nil {
		log.Printf("mp inbound message archive failed: tenant_id=%d account_id=%d: %v", acc.TenantID, acc.ID, err)
		c.String(http.StatusInternalServerError, "message archive failed")
		return
	}
	if !inserted {
		c.String(http.StatusOK, "success")
		return
	}

	// M5 发布结果事件：freepublish 完成后微信推 PUBLISHJOBFINISH，按 publish_id
	// 回写文章状态。此类事件无需回复。
	if isPublishResult {
		c.String(http.StatusOK, "success")
		return
	}

	reply := s.buildReply(acc, in)
	if reply == "" {
		c.String(http.StatusOK, "success")
		return
	}

	from := acc.GhID
	if from == "" {
		from = in.ToUserName
	}
	replyXML := wxmsg.BuildTextReply(in.FromUserName, from, reply)

	if err := s.Store.LogMessage(&store.MpMessage{
		TenantID: acc.TenantID, AccountID: acc.ID,
		OpenID: in.FromUserName, Direction: "out",
		MsgType: "text", Content: reply,
	}); err != nil {
		// The passive reply has not crossed an external API boundary yet, but
		// returning 500 here would make the archived inbound row suppress the
		// retry. Keep delivery, and make the partial archive failure observable.
		log.Printf("mp passive reply archive failed: tenant_id=%d account_id=%d: %v", acc.TenantID, acc.ID, err)
	}

	// 安全模式：回复也要加密 + 重签名。
	if aesCipher != nil {
		var nonce16 [16]byte
		if _, rerr := rand.Read(nonce16[:]); rerr != nil {
			c.String(http.StatusOK, "success")
			return
		}
		encrypted, eerr := aesCipher.Encrypt([]byte(replyXML), nonce16[:])
		if eerr != nil {
			c.String(http.StatusOK, "success")
			return
		}
		msgSig := wxcrypto.MsgSignature(acc.Token, timestamp, nonce, encrypted)
		out := wxmsg.BuildEncryptedReply(encrypted, msgSig, timestamp, nonce)
		c.Header("Content-Type", "application/xml; charset=utf-8")
		c.String(http.StatusOK, out)
		return
	}

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.String(http.StatusOK, replyXML)
}

func inboundMessageTime(createTime int64, now time.Time) time.Time {
	if createTime <= 0 {
		return now
	}
	createdAt := time.Unix(createTime, 0)
	if createdAt.After(now.Add(5 * time.Minute)) {
		return now
	}
	return createdAt
}

// handlePublishResult 处理 PUBLISHJOBFINISH。两条发布记录路径都严格限定在
// 已验签公众号所属的 tenant/account 下；任一数据库错误都向上传递以触发微信重试。
func (s *Server) handlePublishResult(acc *store.MpAccount, publishID, publishStatus string) error {
	if acc == nil || publishID == "" || publishStatus == "" {
		return errors.New("invalid publish result payload")
	}
	job, err := s.Store.UpsertPublishResultJob(&store.MpPublishResultJob{
		TenantID: acc.TenantID, AccountID: acc.ID,
		PublishID: publishID, PublishStatus: publishStatus,
	})
	if err != nil {
		return err
	}
	matched, err := s.applyPublishResultJob(job)
	if err != nil {
		return err
	}
	if !matched {
		return nil
	}
	return s.Store.FinishPublishResultJob(job.ID)
}

// buildReply M1 简单回复策略：关注事件欢迎语、文本回声。M4 起接 AI/客服。
func (s *Server) buildReply(acc *store.MpAccount, in *wxmsg.Inbound) string {
	switch in.MsgType {
	case "event":
		switch in.Event {
		case "subscribe":
			return "感谢关注 " + acc.Name + "！回复任意内容试试。"
		default:
			return "" // 其他事件（取关/菜单点击等）M1 暂不回复
		}
	case "text":
		return "收到：" + in.Content
	default:
		return "已收到你的消息。"
	}
}
