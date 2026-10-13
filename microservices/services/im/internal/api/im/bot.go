package api

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"log"
	"net/http"
	"strings"
	"time"
)

type transferHumanReq struct {
	Reason string `json:"reason"`
}

// POST /api/v1/im/conversations/:public_id/transfer_human（转人工）
func (s *Server) TransferHuman(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if conv.VisitorID != guest.VisitorID {
		Fail(c, http.StatusForbidden, "not your conversation")
		return
	}
	var req transferHumanReq
	_ = c.ShouldBindJSON(&req)
	reason := req.Reason
	if reason == "" {
		reason = "visitor"
	}
	if err := s.Store.TransferToHuman(conv, reason); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	// 通知访客与坐席
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "queue.updated", "payload": gin.H{}})
	s.pushTransferInbox(conv, reason)
	// 向访客推送一条简短的 bot/系统提示
	note := &model.Message{
		ConversationID: conv.ID,
		SenderType:     "bot",
		MsgType:        "text",
		Content:        store.JSONText(map[string]any{"text": "已为您转接人工客服，请稍候…"}),
	}
	if err := s.Store.CreateMessage(note); err == nil {
		s.Hub.Publish(conv.PublicID.String(), gin.H{
			"type": "message.new",
			"payload": gin.H{
				"message":                note,
				"conversation_public_id": conv.PublicID.String(),
			},
		})
	}
	OK(c, conv)
}

// pushTransferInbox 给在线/忙碌坐席写站内信（notify-service）；失败只打日志不挡主流程。
func (s *Server) pushTransferInbox(conv *model.Conversation, reason string) {
	if s.Notify == nil || !s.Notify.Enabled() || conv == nil {
		return
	}
	// 只推给本会话所属租户的坐席（原来是全平台在线坐席）
	agents, err := s.Store.ListPresence(conv.TenantID, "online", "busy")
	if err != nil || len(agents) == 0 {
		return
	}
	ids := make([]uint64, 0, len(agents))
	for _, a := range agents {
		if a.AgentUserID > 0 {
			ids = append(ids, a.AgentUserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if reason == "" {
		reason = "transfer"
	}
	publicID := conv.PublicID.String()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, err := s.Notify.Send(ctx, notifyclient.SendInput{
			TenantID:     conv.TenantID,
			UserIDs:      ids,
			TemplateCode: "im.transfer_human",
			Type:         "im",
			RefType:      "im_conversation",
			RefID:        publicID,
			Vars: map[string]string{
				"public_id": publicID,
				"reason":    reason,
			},
		})
		if err != nil {
			log.Printf("im transfer notify: %v", err)
		}
	}()
}

// POST /api/v1/im/agent/conversations/:public_id/summary（生成会话小结）
func (s *Server) AgentSummary(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	summary, err := s.generateSummary(c.Request.Context(), conv)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.SaveSummary(conv, summary); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "summary", map[string]any{"summary": summary})
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	OK(c, gin.H{"summary": summary, "conversation": conv})
}

// afterVisitorMessage 执行转人工意图检查与 bot 回复（异步）。
func (s *Server) afterVisitorMessage(conv *model.Conversation, text string) {
	if conv == nil || conv.Status != "bot_serving" {
		return
	}
	// 图片/文件等非文本消息不触发机器人（模型只支持文本）
	if strings.TrimSpace(text) == "" {
		return
	}
	// 为 goroutine 复制 public id
	publicID := conv.PublicID.String()
	convID := conv.ID
	siteID := conv.SiteID
	go func() {
		// 重新获取最新状态
		c, err := s.Store.GetConversationByPublicID(publicID)
		if err != nil || c.Status != "bot_serving" {
			return
		}
		if bot.WantsHuman(text) {
			if err := s.Store.TransferToHuman(c, "keyword"); err != nil {
				return
			}
			if refreshed, err := s.Store.GetConversationByPublicID(publicID); err == nil {
				c = refreshed
			}
			s.Hub.Publish(publicID, gin.H{"type": "conversation.updated", "payload": c})
			s.AgentHub.Publish(c.TenantID, gin.H{"type": "conversation.updated", "payload": c})
			s.AgentHub.Publish(c.TenantID, gin.H{"type": "queue.updated", "payload": gin.H{}})
			s.pushTransferInbox(c, "keyword")
			note := &model.Message{
				ConversationID: convID,
				SenderType:     "bot",
				MsgType:        "text",
				Content:        store.JSONText(map[string]any{"text": "好的，正在为您转接人工客服，请稍候…"}),
			}
			if err := s.Store.CreateMessage(note); err == nil {
				s.Hub.Publish(publicID, gin.H{
					"type":    "message.new",
					"payload": gin.H{"message": note, "conversation_public_id": publicID},
				})
			}
			return
		}
		if s.Bot == nil {
			return
		}
		reply, hits, kbMode, err := s.botReply(context.Background(), siteID, convID, publicID, text)
		if err != nil {
			log.Printf("im bot reply: %v", err)
			// 降级：转人工兜底
			reply = "抱歉，智能助手暂时无法回答。您可以回复「转人工」接入坐席。"
			kbMode = "error"
		}
		// 若模型运行期间访客已转人工则跳过
		if latest, err := s.Store.GetConversationByPublicID(publicID); err != nil || latest.Status != "bot_serving" {
			return
		}
		payload := map[string]any{"text": reply}
		if kbMode != "" {
			payload["kb_mode"] = kbMode
		}
		if len(hits) > 0 {
			cite := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				cite = append(cite, map[string]any{
					"document_id": h.DocumentID,
					"title":       h.Title,
					"snippet":     h.Snippet,
					"score":       h.Score,
				})
			}
			payload["kb_hits"] = cite
		}
		// kb_source 在 botReply 中经旁路通道填充 —— 仅附带模式（vector|keyword 记入 stats）
		msg := &model.Message{
			ConversationID: convID,
			SenderType:     "bot",
			MsgType:        "text",
			Content:        store.JSONText(payload),
		}
		if err := s.Store.CreateMessage(msg); err != nil {
			log.Printf("im bot save: %v", err)
			return
		}
		s.Hub.Publish(publicID, gin.H{
			"type":    "message.new",
			"payload": gin.H{"message": msg, "conversation_public_id": publicID},
		})
		if refreshed, err := s.Store.GetConversationByPublicID(publicID); err == nil {
			s.AgentHub.Publish(refreshed.TenantID, gin.H{"type": "conversation.updated", "payload": refreshed})
		}
	}()
}

// botReply 构建历史，可选检索知识库片段用于 RAG，
// 返回 (reply, hits, kbMode, err)。kbMode：rag | miss | off | none。
// 优先向量检索（ai-service），其次关键词兜底（M1.5）。
func (s *Server) botReply(ctx context.Context, siteID, convID uint64, publicID, lastVisitorText string) (string, []store.KBHit, string, error) {
	system := s.BotSystemPrompt
	var site *model.Site
	if st, err := s.Store.GetSite(siteID); err == nil {
		site = st
		if strings.TrimSpace(site.BotSystemPrompt) != "" {
			system = site.BotSystemPrompt
		}
	}
	kbOn := site == nil || site.BotKBEnabled
	tenantID := uint64(1)
	visitorID := uint64(0)
	if conv, err := s.Store.GetConversationByPublicID(publicID); err == nil {
		tenantID = conv.TenantID
		visitorID = conv.VisitorID
	}

	var hits []store.KBHit
	kbMode := "off"
	kbSource := "" // vector | keyword
	if kbOn {
		kbMode = "none"
		// M1.5：优先 ai-service 向量检索，失败/无命中再降级关键词
		if s.KBVector != nil && s.KBVector.Enabled() {
			vctx, vcancel := context.WithTimeout(ctx, 10*time.Second)
			vhits, verr := s.KBVector.SearchVector(vctx, tenantID, lastVisitorText, 3)
			vcancel()
			if verr != nil {
				log.Printf("im kb vector: %v", verr)
				s.Store.RecordKBSearch(tenantID, "vector_error")
			} else if len(vhits) > 0 {
				hits = vhits
				kbSource = "vector"
			}
		}
		if len(hits) == 0 {
			found, err := s.Store.SearchKnowledge(tenantID, lastVisitorText, 3)
			if err != nil {
				log.Printf("im kb keyword: %v", err)
			} else if len(found) > 0 {
				hits = found
				kbSource = "keyword"
			}
		}
		if len(hits) > 0 {
			kbMode = "rag"
			system = store.BuildKBSystemPrompt(system, hits)
			if kbSource != "" {
				s.Store.RecordKBSearch(tenantID, kbSource)
			}
		} else {
			kbMode = "miss"
			s.Store.RecordKBSearch(tenantID, "miss")
			_ = s.Store.CreateKBMiss(&model.KBMiss{
				TenantID:             tenantID,
				SiteID:               siteID,
				ConversationPublicID: publicID,
				VisitorID:            visitorID,
				Question:             lastVisitorText,
				Status:               "open",
			})
			if strings.TrimSpace(system) == "" {
				system = "你是企业在线客服助手，回答简洁、礼貌、可用中文。"
			}
			system += "\n\n当前知识库未检索到相关内容。请勿编造具体业务数据；若无法确定请明确说明并建议访客回复「转人工」。"
		}
	}

	msgs, err := s.Store.ListMessages(convID, 0, 30)
	if err != nil {
		return "", hits, kbMode, err
	}
	history := make([]bot.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.MsgType == "event" {
			continue
		}
		text := bot.ExtractText(m.Content)
		if text == "" {
			continue
		}
		role := m.SenderType
		switch role {
		case "visitor":
			role = "user"
		case "bot", "agent":
			role = "assistant"
		default:
			continue
		}
		history = append(history, bot.Message{Role: role, Content: text})
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	reply, err := s.Bot.Complete(ctx, system, history)
	return reply, hits, kbMode, err
}

func (s *Server) generateSummary(ctx context.Context, conv *model.Conversation) (string, error) {
	if s.Bot == nil {
		return "", nil
	}
	msgs, err := s.Store.ListMessages(conv.ID, 0, 50)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("请用中文写一段简短客服会话小结（3～6 句），含：访客诉求、处理过程、结果/待办。\n\n对话：\n")
	for _, m := range msgs {
		if m.MsgType == "event" {
			continue
		}
		text := bot.ExtractText(m.Content)
		if text == "" {
			continue
		}
		b.WriteString(m.SenderType)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n")
	}
	history := []bot.Message{{Role: "user", Content: b.String()}}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	return s.Bot.Complete(ctx, "你是客服质检助手，只输出小结正文。", history)
}
