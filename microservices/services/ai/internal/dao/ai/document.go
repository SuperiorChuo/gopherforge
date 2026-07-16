package ai

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg/pagination"
	"github.com/go-admin-kit/services/ai/internal/pkg/tenant"
)

// DocumentDAO persists knowledge-base documents and their embedded chunks.
// The pgvector embedding column is written and queried through raw SQL
// because GORM has no native vector type support.
type DocumentDAO struct {
	db *gorm.DB
}

// NewDocumentDAO builds a DocumentDAO backed by an injected handle.
func NewDocumentDAO(db *gorm.DB) *DocumentDAO {
	return &DocumentDAO{db: db}
}

func (d *DocumentDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// CreateContext inserts a document row.
func (d *DocumentDAO) CreateContext(ctx context.Context, document *model.AIDocument) error {
	return d.dbWithContext(ctx).Create(document).Error
}

// GetContext loads one document within the tenant.
func (d *DocumentDAO) GetContext(ctx context.Context, id uint) (*model.AIDocument, error) {
	var document model.AIDocument
	result := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		First(&document)
	return &document, result.Error
}

// UpdateChunkCountContext stores the final chunk count of a document within
// the tenant.
func (d *DocumentDAO) UpdateChunkCountContext(ctx context.Context, documentID uint, chunkCount int) error {
	return d.dbWithContext(ctx).
		Model(&model.AIDocument{}).
		Where("id = ? AND tenant_id = ?", documentID, tenant.FromContextOrDefault(ctx)).
		Update("chunk_count", chunkCount).Error
}

// ListContext returns one page of documents for the tenant, newest first.
func (d *DocumentDAO) ListContext(ctx context.Context, req pagination.PageRequest) ([]model.AIDocument, int64, error) {
	var documents []model.AIDocument
	var total int64

	query := d.dbWithContext(ctx).Model(&model.AIDocument{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&documents)
	return documents, total, result.Error
}

// CountContext returns the number of stored documents for the tenant.
func (d *DocumentDAO) CountContext(ctx context.Context) (int64, error) {
	var total int64
	err := d.dbWithContext(ctx).Model(&model.AIDocument{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Count(&total).Error
	return total, err
}

// DeleteContext deletes a document within the tenant; chunks cascade at the
// database level.
func (d *DocumentDAO) DeleteContext(ctx context.Context, id uint) error {
	result := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Delete(&model.AIDocument{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// InsertChunkContext inserts one chunk with its embedding via raw SQL. A nil
// embedding stores SQL NULL so the chunk is excluded from similarity search.
func (d *DocumentDAO) InsertChunkContext(ctx context.Context, chunk *model.AIDocumentChunk, embedding []float32) error {
	if embedding == nil {
		return d.dbWithContext(ctx).Exec(
			`INSERT INTO ai_document_chunks (document_id, chunk_index, content, embedding) VALUES ($1, $2, $3, NULL)`,
			chunk.DocumentID, chunk.ChunkIndex, chunk.Content,
		).Error
	}
	return d.dbWithContext(ctx).Exec(
		`INSERT INTO ai_document_chunks (document_id, chunk_index, content, embedding) VALUES ($1, $2, $3, $4::vector)`,
		chunk.DocumentID, chunk.ChunkIndex, chunk.Content, EncodeVector(embedding),
	).Error
}

// ChunkMatch is one similarity-search hit joined with its document title.
type ChunkMatch struct {
	DocumentID uint    `json:"document_id"`
	Title      string  `json:"title"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

// SearchChunksContext runs an exact cosine similarity scan over the tenant's
// embedded chunks and returns the topK closest matches. Score is 1 - cosine
// distance, so higher is more similar.
func (d *DocumentDAO) SearchChunksContext(ctx context.Context, embedding []float32, topK int) ([]ChunkMatch, error) {
	if topK <= 0 {
		topK = 5
	}
	var matches []ChunkMatch
	err := d.dbWithContext(ctx).Raw(
		`SELECT c.document_id, d.title, c.chunk_index, c.content,
		        1 - (c.embedding <=> $1::vector) AS score
		 FROM ai_document_chunks c
		 JOIN ai_documents d ON d.id = c.document_id
		 WHERE c.embedding IS NOT NULL AND d.tenant_id = $2
		 ORDER BY c.embedding <=> $1::vector
		 LIMIT $3`,
		EncodeVector(embedding), tenant.FromContextOrDefault(ctx), topK,
	).Scan(&matches).Error
	return matches, err
}

// EncodeVector serializes a float32 slice into the pgvector text literal
// format, e.g. "[1,2,3]".
func EncodeVector(vector []float32) string {
	var b strings.Builder
	b.Grow(len(vector)*10 + 2)
	b.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
