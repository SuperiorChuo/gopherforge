package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
	"github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg/pagination"
	"github.com/go-admin-kit/services/ai/internal/pkg/tenant"
)

// ErrDocumentNotFound reports a missing knowledge-base document.
var ErrDocumentNotFound = errors.New("document not found")

const (
	// chunkTargetSize is the approximate chunk size in characters.
	chunkTargetSize = 500
	// chunkOverlap is the number of trailing characters carried into the
	// next chunk for context continuity.
	chunkOverlap = 50
	// embedBatchSize bounds one embedding API call.
	embedBatchSize = 16
)

// KnowledgeService manages knowledge-base documents: chunking, embedding,
// and similarity search.
type KnowledgeService struct {
	documents *aidao.DocumentDAO
	providers aiclient.Providers
}

// NewKnowledgeServiceWithDB builds a KnowledgeService backed by an injected
// database handle and provider set.
func NewKnowledgeServiceWithDB(db *gorm.DB, providers aiclient.Providers) KnowledgeService {
	return KnowledgeService{
		documents: aidao.NewDocumentDAO(db),
		providers: providers,
	}
}

// CreateDocumentResult reports the stored document and its chunk count.
type CreateDocumentResult struct {
	ID         uint `json:"id"`
	ChunkCount int  `json:"chunk_count"`
}

// CreateDocument stores a document, splits it into chunks, embeds them in
// batches, and persists the chunks with their embeddings.
func (s *KnowledgeService) CreateDocument(ctx context.Context, title, content string, uploaderID uint) (*CreateDocumentResult, error) {
	now := time.Now()
	document := &model.AIDocument{
		TenantID:   tenant.FromContextOrDefault(ctx),
		Title:      title,
		Content:    content,
		UploaderID: uploaderID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.documents.CreateContext(ctx, document); err != nil {
		return nil, err
	}

	chunks := SplitIntoChunks(content, chunkTargetSize, chunkOverlap)
	for start := 0; start < len(chunks); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		vectors, err := s.providers.Embed.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed document chunks: %w", err)
		}
		if len(vectors) != len(batch) {
			return nil, fmt.Errorf("embedding provider returned %d vectors for %d chunks", len(vectors), len(batch))
		}

		for i, chunkContent := range batch {
			chunk := &model.AIDocumentChunk{
				DocumentID: document.ID,
				ChunkIndex: start + i,
				Content:    chunkContent,
			}
			if err := s.documents.InsertChunkContext(ctx, chunk, vectors[i]); err != nil {
				return nil, err
			}
		}
	}

	if err := s.documents.UpdateChunkCountContext(ctx, document.ID, len(chunks)); err != nil {
		return nil, err
	}
	return &CreateDocumentResult{ID: document.ID, ChunkCount: len(chunks)}, nil
}

// ListDocuments returns one page of documents.
func (s *KnowledgeService) ListDocuments(ctx context.Context, req pagination.PageRequest) ([]model.AIDocument, int64, error) {
	return s.documents.ListContext(ctx, req)
}

// CountDocuments returns the number of stored documents.
func (s *KnowledgeService) CountDocuments(ctx context.Context) (int64, error) {
	return s.documents.CountContext(ctx)
}

// GetDocument loads one knowledge-base document within the tenant.
func (s *KnowledgeService) GetDocument(ctx context.Context, id uint) (*model.AIDocument, error) {
	document, err := s.documents.GetContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return document, nil
}

// DeleteDocument removes a document and its chunks within the tenant.
func (s *KnowledgeService) DeleteDocument(ctx context.Context, id uint) error {
	err := s.documents.DeleteContext(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDocumentNotFound
	}
	return err
}

// Search embeds the query and returns the topK most similar chunks.
func (s *KnowledgeService) Search(ctx context.Context, query string, topK int) ([]aidao.ChunkMatch, error) {
	vectors, err := s.providers.Embed.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return s.documents.SearchChunksContext(ctx, vectors[0], topK)
}

// SplitIntoChunks aggregates paragraphs into chunks of roughly targetSize
// characters, carrying overlap trailing characters between chunks. A
// paragraph longer than targetSize is split hard at targetSize boundaries.
func SplitIntoChunks(content string, targetSize, overlap int) []string {
	if targetSize <= 0 {
		targetSize = chunkTargetSize
	}
	if overlap < 0 || overlap >= targetSize {
		overlap = 0
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	paragraphs := strings.Split(content, "\n\n")

	var pieces []string
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > targetSize {
			pieces = append(pieces, string(runes[:targetSize]))
			runes = runes[targetSize:]
		}
		if len(runes) > 0 {
			pieces = append(pieces, string(runes))
		}
	}

	var chunks []string
	var current []rune
	for _, piece := range pieces {
		pieceRunes := []rune(piece)
		if len(current) > 0 && len(current)+len(pieceRunes)+2 > targetSize {
			chunks = append(chunks, string(current))
			if overlap > 0 && len(current) > overlap {
				tail := make([]rune, overlap)
				copy(tail, current[len(current)-overlap:])
				current = tail
			} else {
				current = current[:0]
			}
		}
		if len(current) > 0 {
			current = append(current, '\n', '\n')
		}
		current = append(current, pieceRunes...)
	}
	if len(current) > 0 {
		chunks = append(chunks, string(current))
	}
	return chunks
}
