package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg"
	"github.com/go-admin-kit/services/internal/testdb"
	"gorm.io/gorm"
)

func modelConfigFixture(t *testing.T) (*AIModelService, *dao.AIModelDAO, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t, &localmodel.AIModel{})
	encryptor, err := pkg.NewEncryptor(strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	repository := dao.NewAIModelDAO(db)
	return NewAIModelService(repository, encryptor), repository, db
}

func TestCreateModelAcceptsUnusedNameAndEncryptsKey(t *testing.T) {
	s, repository, _ := modelConfigFixture(t)
	ctx := context.Background()
	const testPlaintextKey = "TEST_MODEL_CONFIG_API_KEY"
	m := &localmodel.AIModel{Name: "new-release-model", Provider: "openai", APIKey: testPlaintextKey, Status: 1}
	if err := s.CreateModel(ctx, m); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetByID(ctx, m.ID)
	if err != nil || stored.APIKey == "" || stored.APIKey == testPlaintextKey || stored.APIKeyIV == "" {
		t.Fatalf("missing encrypted persistence: %v", err)
	}
	internal, err := s.GetModel(ctx, m.ID)
	if err != nil || internal.APIKey != testPlaintextKey {
		t.Fatalf("decrypt round trip failed: %v", err)
	}
	display, err := s.GetModelForDisplay(ctx, m.ID)
	if err != nil || display.APIKey != "********" || display.APIKeyIV != "" {
		t.Fatalf("display leaked model credential: %v", err)
	}
}

func TestModelConfigKeepsDuplicateNameProtection(t *testing.T) {
	s, repository, db := modelConfigFixture(t)
	ctx := context.Background()
	first := &localmodel.AIModel{Name: "existing", Provider: "openai", Status: 1}
	second := &localmodel.AIModel{Name: "other", Provider: "openai", Status: 1}
	for _, m := range []*localmodel.AIModel{first, second} {
		if err := repository.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateModel(ctx, &localmodel.AIModel{Name: first.Name}); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("duplicate creation was not rejected: %v", err)
	}
	if err := s.UpdateModel(ctx, second.ID, &localmodel.AIModel{Name: first.Name}); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("duplicate rename was not rejected: %v", err)
	}
	unchanged, err := repository.GetByID(ctx, second.ID)
	if err != nil || unchanged.Name != second.Name {
		t.Fatalf("rejected rename modified row: %v", err)
	}
	var count int64
	if err := db.Model(&localmodel.AIModel{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("row count=%d, want 2: %v", count, err)
	}
}

func TestModelConfigCanRenameToUnusedName(t *testing.T) {
	s, repository, _ := modelConfigFixture(t)
	ctx := context.Background()
	m := &localmodel.AIModel{Name: "original", Provider: "openai", Status: 1}
	if err := repository.Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"renamed", "renamed"} { // New name, then an unchanged name.
		if err := s.UpdateModel(ctx, m.ID, &localmodel.AIModel{Name: name, DisplayName: "updated"}); err != nil {
			t.Fatal(err)
		}
		got, err := repository.GetByID(ctx, m.ID)
		if err != nil || got.Name != name || got.DisplayName != "updated" {
			t.Fatalf("rename did not persist: %v", err)
		}
	}
}

func TestModelConfigDoesNotIgnoreLookupFailure(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "rename"}[rename], func(t *testing.T) {
			s, repository, db := modelConfigFixture(t)
			ctx := context.Background()
			m := &localmodel.AIModel{Name: "original", Provider: "openai", Status: 1}
			if err := repository.Create(ctx, m); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected model name lookup failure")
			const hook = "test:model_name_lookup_failure"
			queries := 0
			if err := db.Callback().Query().Before("gorm:query").Register(hook, func(q *gorm.DB) {
				queries++
				if !rename || queries == 2 { // Update first resolves the existing ID.
					q.AddError(injected)
				}
			}); err != nil {
				t.Fatal(err)
			}
			var err error
			if rename {
				err = s.UpdateModel(ctx, m.ID, &localmodel.AIModel{Name: "unused"})
			} else {
				err = s.CreateModel(ctx, &localmodel.AIModel{Name: "unused"})
			}
			if removeErr := db.Callback().Query().Remove(hook); removeErr != nil {
				t.Fatal(removeErr)
			}
			if !errors.Is(err, injected) {
				t.Fatalf("lookup failure not propagated: %v", err)
			}
			got, err := repository.GetByID(ctx, m.ID)
			if err != nil || got.Name != "original" {
				t.Fatalf("lookup failure changed existing row: %v", err)
			}
			var count int64
			if err := db.Model(&localmodel.AIModel{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("lookup failure changed row count=%d: %v", count, err)
			}
		})
	}
}
