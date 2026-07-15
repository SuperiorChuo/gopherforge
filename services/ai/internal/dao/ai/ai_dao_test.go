package ai

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg/pagination"
)

func setupAIDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}

func TestConversationDAOGetForUserContextScopesByUser(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_conversations" WHERE id = $1 AND user_id = $2 ORDER BY "ai_conversations"."id" LIMIT $3`)).
		WithArgs(uint(7), uint(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title"}).AddRow(uint(7), uint(3), "hello"))

	conversation, err := NewConversationDAO(db).GetForUserContext(context.Background(), 7, 3)
	if err != nil {
		t.Fatalf("GetForUserContext() error = %v", err)
	}
	if conversation.ID != 7 || conversation.UserID != 3 {
		t.Fatalf("GetForUserContext() = %+v, want id=7 user_id=3", conversation)
	}
}

func TestConversationDAODeleteForUserContextReturnsNotFound(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "ai_conversations" WHERE id = $1 AND user_id = $2`)).
		WithArgs(uint(7), uint(3)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewConversationDAO(db).DeleteForUserContext(context.Background(), 7, 3)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteForUserContext() error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestConversationDAOListForUserContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupAIDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewConversationDAO(db).ListForUserContext(ctx, 3, pagination.PageRequest{Page: 1, PageSize: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListForUserContext() error = %v, want context.Canceled", err)
	}
}

func TestMessageDAOListRecentByConversationContextReversesToChronological(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_messages" WHERE conversation_id = $1 ORDER BY id DESC LIMIT $2`)).
		WithArgs(uint(9), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "role", "content"}).
			AddRow(uint(3), uint(9), "assistant", "second reply").
			AddRow(uint(2), uint(9), "user", "second question").
			AddRow(uint(1), uint(9), "user", "first question"))

	messages, err := NewMessageDAO(db).ListRecentByConversationContext(context.Background(), 9, 20)
	if err != nil {
		t.Fatalf("ListRecentByConversationContext() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("ListRecentByConversationContext() len = %d, want 3", len(messages))
	}
	if messages[0].ID != 1 || messages[2].ID != 3 {
		t.Fatalf("ListRecentByConversationContext() order = [%d %d %d], want chronological", messages[0].ID, messages[1].ID, messages[2].ID)
	}
}

func TestDocumentDAOInsertChunkContextEncodesVectorLiteral(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO ai_document_chunks (document_id, chunk_index, content, embedding) VALUES ($1, $2, $3, $4::vector)`)).
		WithArgs(uint(5), 0, "chunk text", "[0.5,-1,2]").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDocumentDAO(db).InsertChunkContext(context.Background(), &model.AIDocumentChunk{
		DocumentID: 5,
		ChunkIndex: 0,
		Content:    "chunk text",
	}, []float32{0.5, -1, 2})
	if err != nil {
		t.Fatalf("InsertChunkContext() error = %v", err)
	}
}

func TestDocumentDAOSearchChunksContextOrdersByCosineDistance(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`ORDER BY c.embedding <=> $1::vector`)).
		WithArgs("[1,0]", 5).
		WillReturnRows(sqlmock.NewRows([]string{"document_id", "title", "chunk_index", "content", "score"}).
			AddRow(uint(2), "doc", 0, "match", 0.92))

	matches, err := NewDocumentDAO(db).SearchChunksContext(context.Background(), []float32{1, 0}, 5)
	if err != nil {
		t.Fatalf("SearchChunksContext() error = %v", err)
	}
	if len(matches) != 1 || matches[0].DocumentID != 2 || matches[0].Score != 0.92 {
		t.Fatalf("SearchChunksContext() matches = %+v", matches)
	}
}

func TestDocumentDAODeleteContextReturnsNotFound(t *testing.T) {
	db, mock := setupAIDAOTestDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "ai_documents" WHERE id = $1`)).
		WithArgs(uint(4)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewDocumentDAO(db).DeleteContext(context.Background(), 4)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteContext() error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestEncodeVector(t *testing.T) {
	got := EncodeVector([]float32{1, -0.25, 3.5})
	want := "[1,-0.25,3.5]"
	if got != want {
		t.Fatalf("EncodeVector() = %q, want %q", got, want)
	}
	if EncodeVector(nil) != "[]" {
		t.Fatalf("EncodeVector(nil) = %q, want []", EncodeVector(nil))
	}
}

func TestInsightDAOLoginStatsSinceContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupAIDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewInsightDAO(db).LoginStatsSinceContext(ctx, timeNowForInsightTest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoginStatsSinceContext() error = %v, want context.Canceled", err)
	}
}

func timeNowForInsightTest() time.Time {
	return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
}
