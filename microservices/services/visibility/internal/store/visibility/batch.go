package store

import (
	"context"
	"errors"
	"strings"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/gorm"
)

// Batch imports are one transaction, including the dedupe read and audit
// callbacks. This is request atomicity, not concurrent uniqueness: the schema
// intentionally has no case-insensitive unique index for these candidates.
func (s *Store) batch(ctx context.Context, fn func(*Store) (int, int, error)) (int, int, error) {
	var created, skipped int
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, skipped, err = fn(&Store{db: tx, idClient: s.idClient})
		return err
	})
	if err != nil {
		return 0, 0, errors.Join(err, ctx.Err())
	}
	return created, skipped, nil
}

func (s *Store) BatchCreateBrands(ctx context.Context, tenantID uint64, items []model.Brand) (int, int, error) {
	return s.batch(ctx, func(tx *Store) (int, int, error) {
		seen, err := tx.ListBrandNames(tenantID)
		if err != nil {
			return 0, 0, err
		}
		created, skipped := 0, 0
		for _, it := range items {
			name := strings.TrimSpace(it.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				skipped++
				continue
			}
			kind := strings.TrimSpace(it.Kind)
			if kind != "brand" {
				kind = "competitor"
			}
			b := &model.Brand{TenantID: tenantID, Name: name, Aliases: strings.TrimSpace(it.Aliases), Kind: kind, Website: strings.TrimSpace(it.Website), Note: strings.TrimSpace(it.Note), Status: 1}
			if err := tx.CreateBrand(ctx, b); err != nil {
				return 0, 0, err
			}
			seen[key] = true
			for _, alias := range model.SplitAliases(b.Aliases) {
				seen[strings.ToLower(alias)] = true
			}
			created++
		}
		return created, skipped, nil
	})
}

func (s *Store) BatchCreateKeywords(ctx context.Context, tenantID uint64, items []model.Keyword) (int, int, error) {
	return s.batch(ctx, func(tx *Store) (int, int, error) {
		existing, err := tx.ListKeywordTexts(tenantID)
		if err != nil {
			return 0, 0, err
		}
		seen := batchSeen(existing)
		created, skipped := 0, 0
		for _, it := range items {
			text := strings.TrimSpace(it.Keyword)
			if text == "" {
				continue
			}
			key := strings.ToLower(text)
			if seen[key] {
				skipped++
				continue
			}
			locale := strings.TrimSpace(it.Locale)
			if locale == "" {
				locale = "zh-CN"
			}
			k := &model.Keyword{TenantID: tenantID, Keyword: text, Locale: locale, TargetURL: strings.TrimSpace(it.TargetURL), Status: "tracking"}
			if err := tx.CreateKeyword(ctx, k); err != nil {
				return 0, 0, err
			}
			seen[key] = true
			created++
		}
		return created, skipped, nil
	})
}

func (s *Store) BatchCreateGeoPrompts(ctx context.Context, tenantID uint64, items []model.GeoPrompt) (int, int, error) {
	return s.batch(ctx, func(tx *Store) (int, int, error) {
		existing, err := tx.ListGeoPromptQuestions(tenantID)
		if err != nil {
			return 0, 0, err
		}
		seen := batchSeen(existing)
		created, skipped := 0, 0
		for _, it := range items {
			text := strings.TrimSpace(it.Question)
			if text == "" {
				continue
			}
			key := strings.ToLower(text)
			if seen[key] {
				skipped++
				continue
			}
			category := strings.TrimSpace(it.Category)
			if category == "" {
				category = "category"
			}
			engine := strings.TrimSpace(it.Engine)
			if engine == "" {
				engine = "llm"
			}
			p := &model.GeoPrompt{TenantID: tenantID, Question: text, Category: category, Engine: engine, Status: 1}
			if err := tx.CreateGeoPrompt(ctx, p); err != nil {
				return 0, 0, err
			}
			seen[key] = true
			created++
		}
		return created, skipped, nil
	})
}

func batchSeen(texts []string) map[string]bool {
	seen := make(map[string]bool, len(texts))
	for _, text := range texts {
		seen[strings.ToLower(strings.TrimSpace(text))] = true
	}
	return seen
}
