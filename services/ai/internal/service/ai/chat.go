// Package ai orchestrates chat conversations, knowledge-base retrieval, and
// AI-generated reports on top of the provider abstraction.
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
	"github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg/pagination"
)

// ErrConversationNotFound reports a missing or foreign conversation.
var ErrConversationNotFound = errors.New("conversation not found")

const (
	// conversationTitleMaxRunes bounds auto-generated conversation titles.
	conversationTitleMaxRunes = 30
	// chatHistoryLimit bounds the conversation history sent to the model.
	chatHistoryLimit = 20
	// knowledgeBaseTopK is the number of chunks retrieved for RAG prompts.
	knowledgeBaseTopK = 5
)

// ChatService manages conversations and streams model completions.
type ChatService struct {
	conversations *aidao.ConversationDAO
	messages      *aidao.MessageDAO
	documents     *aidao.DocumentDAO
	providers     aiclient.Providers
}

// NewChatServiceWithDB builds a ChatService backed by an injected database
// handle and provider set.
func NewChatServiceWithDB(db *gorm.DB, providers aiclient.Providers) ChatService {
	return ChatService{
		conversations: aidao.NewConversationDAO(db),
		messages:      aidao.NewMessageDAO(db),
		documents:     aidao.NewDocumentDAO(db),
		providers:     providers,
	}
}

// ChatRequest is the resolved input of one chat turn.
type ChatRequest struct {
	ConversationID   uint
	UserID           uint
	Message          string
	UseKnowledgeBase bool
}

// ChatResult reports the persisted identifiers of a completed chat turn.
type ChatResult struct {
	ConversationID uint
	MessageID      uint
}

// StreamChat runs one chat turn: it resolves or creates the conversation,
// loads recent history, optionally augments the prompt with knowledge-base
// context, streams the completion through onDelta, and persists both the
// user message and the full assistant reply.
func (s *ChatService) StreamChat(ctx context.Context, req ChatRequest, onDelta func(aiclient.ChatDelta) error) (*ChatResult, error) {
	conversation, err := s.resolveConversation(ctx, req)
	if err != nil {
		return nil, err
	}

	history, err := s.messages.ListRecentByConversationContext(ctx, conversation.ID, chatHistoryLimit)
	if err != nil {
		return nil, err
	}

	msgs := make([]aiclient.ChatMessage, 0, len(history)+2)
	if req.UseKnowledgeBase {
		contextPrompt, err := s.buildKnowledgePrompt(ctx, req.Message)
		if err != nil {
			return nil, err
		}
		if contextPrompt != "" {
			msgs = append(msgs, aiclient.ChatMessage{Role: aiclient.RoleSystem, Content: contextPrompt})
		}
	}
	for _, m := range history {
		msgs = append(msgs, aiclient.ChatMessage{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, aiclient.ChatMessage{Role: aiclient.RoleUser, Content: req.Message})

	userMessage := &model.AIMessage{
		ConversationID: conversation.ID,
		Role:           aiclient.RoleUser,
		Content:        req.Message,
		CreatedAt:      time.Now(),
	}
	if err := s.messages.CreateContext(ctx, userMessage); err != nil {
		return nil, err
	}

	var reply strings.Builder
	err = s.providers.Chat.Chat(ctx, msgs, func(delta aiclient.ChatDelta) error {
		if !delta.Done {
			reply.WriteString(delta.Content)
		}
		return onDelta(delta)
	})
	if err != nil {
		return nil, err
	}

	assistantMessage := &model.AIMessage{
		ConversationID: conversation.ID,
		Role:           aiclient.RoleAssistant,
		Content:        reply.String(),
		CreatedAt:      time.Now(),
	}
	if err := s.messages.CreateContext(ctx, assistantMessage); err != nil {
		return nil, err
	}
	if err := s.conversations.TouchContext(ctx, conversation.ID, time.Now()); err != nil {
		return nil, err
	}

	return &ChatResult{ConversationID: conversation.ID, MessageID: assistantMessage.ID}, nil
}

// resolveConversation loads the requested conversation for the user or
// creates a new one titled from the message.
func (s *ChatService) resolveConversation(ctx context.Context, req ChatRequest) (*model.AIConversation, error) {
	if req.ConversationID != 0 {
		conversation, err := s.conversations.GetForUserContext(ctx, req.ConversationID, req.UserID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrConversationNotFound
			}
			return nil, err
		}
		return conversation, nil
	}

	now := time.Now()
	conversation := &model.AIConversation{
		UserID:    req.UserID,
		Title:     truncateRunes(strings.TrimSpace(req.Message), conversationTitleMaxRunes),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.conversations.CreateContext(ctx, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

// buildKnowledgePrompt embeds the query and folds the top matching chunks
// into a system prompt. It returns an empty string when nothing matches.
func (s *ChatService) buildKnowledgePrompt(ctx context.Context, query string) (string, error) {
	vectors, err := s.providers.Embed.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return "", nil
	}

	matches, err := s.documents.SearchChunksContext(ctx, vectors[0], knowledgeBaseTopK)
	if err != nil {
		return "", fmt.Errorf("search knowledge base: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("你是企业内部管理系统的 AI 助手。请优先根据下面的知识库内容回答用户问题;如果知识库内容与问题无关,再依据你自己的知识回答,并说明该回答不来自知识库。\n\n知识库检索结果:\n")
	for i, match := range matches {
		fmt.Fprintf(&b, "\n[%d] 来源文档:%s\n%s\n", i+1, match.Title, match.Content)
	}
	return b.String(), nil
}

// ListConversations returns one page of the user's conversations.
func (s *ChatService) ListConversations(ctx context.Context, userID uint, req pagination.PageRequest) ([]model.AIConversation, int64, error) {
	return s.conversations.ListForUserContext(ctx, userID, req)
}

// ListMessages returns all messages of a conversation owned by the user.
func (s *ChatService) ListMessages(ctx context.Context, conversationID, userID uint) ([]model.AIMessage, error) {
	if _, err := s.conversations.GetForUserContext(ctx, conversationID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	return s.messages.ListByConversationContext(ctx, conversationID)
}

// DeleteConversation deletes a conversation owned by the user.
func (s *ChatService) DeleteConversation(ctx context.Context, conversationID, userID uint) error {
	err := s.conversations.DeleteForUserContext(ctx, conversationID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrConversationNotFound
	}
	return err
}

// truncateRunes shortens value to maxRunes runes.
func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
