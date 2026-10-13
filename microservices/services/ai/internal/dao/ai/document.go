package ai

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
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
func (d *DocumentDAO) CreateContext(ctx context.Context, document *localmodel.AIDocument) error {
	return d.dbWithContext(ctx).Create(document).Error
}

// GetContext loads one document within the tenant.
func (d *DocumentDAO) GetContext(ctx context.Context, id uint) (*localmodel.AIDocument, error) {
	var document localmodel.AIDocument
	result := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		First(&document)
	return &document, result.Error
}

// UpdateChunkCountContext stores the final chunk count of a document within
// the tenant.
func (d *DocumentDAO) UpdateChunkCountContext(ctx context.Context, documentID uint, chunkCount int) error {
	return d.dbWithContext(ctx).
		Model(&localmodel.AIDocument{}).
		Where("id = ? AND tenant_id = ?", documentID, tenant.FromContextOrDefault(ctx)).
		Update("chunk_count", chunkCount).Error
}

// documentListColumns are the metadata columns returned by the list endpoint.
// AIDocument.Content holds the whole uploaded document (type:text), so
// selecting it for a list page pulls every document body into memory for
// nothing — the detail endpoint (GetContext) still returns all columns.
var documentListColumns = []string{
	"id", "tenant_id", "title", "uploader_id", "chunk_count", "created_at", "updated_at",
}

// ListContext returns one page of documents for the tenant, newest first.
// Content is deliberately excluded; see documentListColumns.
func (d *DocumentDAO) ListContext(ctx context.Context, req pagination.PageRequest) ([]localmodel.AIDocument, int64, error) {
	var documents []localmodel.AIDocument
	var total int64

	query := d.dbWithContext(ctx).Model(&localmodel.AIDocument{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Select(documentListColumns).
		Scopes(pagination.Paginate(req)).
		Order("created_at DESC").
		Find(&documents)
	return documents, total, result.Error
}

// CountContext returns the number of stored documents for the tenant.
func (d *DocumentDAO) CountContext(ctx context.Context) (int64, error) {
	var total int64
	err := d.dbWithContext(ctx).Model(&localmodel.AIDocument{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Count(&total).Error
	return total, err
}

// DeleteContext deletes a document within the tenant; chunks cascade at the
// database level.
func (d *DocumentDAO) DeleteContext(ctx context.Context, id uint) error {
	result := d.dbWithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Delete(&localmodel.AIDocument{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateContext saves title/content/uploader/updated_at of an existing
// document, scoped by id + tenant (the caller must have loaded it first).
func (d *DocumentDAO) UpdateContext(ctx context.Context, document *localmodel.AIDocument) error {
	return d.dbWithContext(ctx).
		Model(&localmodel.AIDocument{}).
		Where("id = ? AND tenant_id = ?", document.ID, document.TenantID).
		Select("title", "content", "uploader_id", "updated_at").
		Updates(map[string]interface{}{
			"title":       document.Title,
			"content":     document.Content,
			"uploader_id": document.UploaderID,
			"updated_at":  document.UpdatedAt,
		}).Error
}

// DeleteChunksContext removes all chunks of a document. Used before
// re-ingesting a document whose content changed.
func (d *DocumentDAO) DeleteChunksContext(ctx context.Context, documentID uint) error {
	return d.dbWithContext(ctx).
		Where("document_id = ?", documentID).
		Delete(&localmodel.AIDocumentChunk{}).Error
}

// ListChunksContext returns the chunks of one document in index order.
func (d *DocumentDAO) ListChunksContext(ctx context.Context, documentID uint) ([]localmodel.AIDocumentChunk, error) {
	return fullread.All[localmodel.AIDocumentChunk](d.dbWithContext(ctx).
		Where("document_id = ?", documentID).
		Order("chunk_index ASC"))
}

// InsertChunkContext inserts one chunk with its embedding via raw SQL. A nil
// embedding stores SQL NULL so the chunk is excluded from similarity search.
func (d *DocumentDAO) InsertChunkContext(ctx context.Context, chunk *localmodel.AIDocumentChunk, embedding []float32) error {
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
