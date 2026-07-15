package model

import "time"

// AIConversation is one chat thread owned by a console user.
type AIConversation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Title     string    `gorm:"size:200" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName maps AIConversation onto ai_conversations.
func (AIConversation) TableName() string {
	return "ai_conversations"
}

// AIMessage is one turn inside a conversation.
type AIMessage struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID uint      `gorm:"index" json:"conversation_id"`
	Role           string    `gorm:"size:20" json:"role"`
	Content        string    `gorm:"type:text" json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName maps AIMessage onto ai_messages.
func (AIMessage) TableName() string {
	return "ai_messages"
}

// AIDocument is one knowledge-base document.
type AIDocument struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Title      string    `gorm:"size:200" json:"title"`
	Content    string    `gorm:"type:text" json:"content"`
	UploaderID uint      `json:"uploader_id"`
	ChunkCount int       `gorm:"default:0" json:"chunk_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName maps AIDocument onto ai_documents.
func (AIDocument) TableName() string {
	return "ai_documents"
}

// AIDocumentChunk maps ai_document_chunks without the pgvector embedding
// column, which GORM cannot marshal; embedding reads and writes go through
// raw SQL in the DAO layer instead.
type AIDocumentChunk struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	DocumentID uint   `gorm:"index" json:"document_id"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `gorm:"type:text" json:"content"`
}

// TableName maps AIDocumentChunk onto ai_document_chunks.
func (AIDocumentChunk) TableName() string {
	return "ai_document_chunks"
}
