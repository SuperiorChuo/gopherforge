package system

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
	"testing"
)

func TestCompleteSetMenuTreeWithPermissions(t *testing.T) {
	db := testdb.Open(t, &model.Menu{}, &model.Permission{}, &model.MenuPermission{})
	const n = fullread.BatchSize*2 + 1
	root := model.Menu{Name: "root", Title: "Root", Status: 1}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	children := make([]model.Menu, n)
	permissions := make([]model.Permission, n)
	for i := range children {
		children[i] = model.Menu{ParentID: root.ID, Name: fmt.Sprint(i), Title: "child", Sort: n - i, Status: 1}
		permissions[i] = model.Permission{Name: fmt.Sprint(i), Code: fmt.Sprintf("fixture:%d", i), Type: 1}
	}
	for _, r := range []any{&children, &permissions} {
		if err := db.CreateInBatches(r, 128).Error; err != nil {
			t.Fatal(err)
		}
	}
	// More than one batch of permissions on ONE menu must not be truncated.
	links := make([]model.MenuPermission, 0, n*2)
	for i := range children {
		links = append(links, model.MenuPermission{MenuID: root.ID, PermissionID: permissions[i].ID}, model.MenuPermission{MenuID: children[i].ID, PermissionID: permissions[0].ID})
	}
	if err := db.CreateInBatches(&links, 128).Error; err != nil {
		t.Fatal(err)
	}
	disabled := model.Menu{Name: "disabled", Title: "disabled", Status: 2}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	d := NewMenuDAO(db)
	status := int8(1)
	ctx := context.Background()
	batches := 0
	const observer = "test:menu_batches"
	if err := db.Callback().Query().After("gorm:query").Register(observer, func(q *gorm.DB) {
		if !q.DryRun && q.Error == nil && q.Statement.Schema != nil && q.Statement.Schema.Table == "menus" {
			if q.RowsAffected > fullread.BatchSize {
				t.Errorf("unbounded menu fetch: %d", q.RowsAffected)
			}
			if q.RowsAffected > 0 {
				batches++
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetMenuTreeContext(ctx, &status)
	db.Callback().Query().Remove(observer)
	if err != nil || len(got) != 1 {
		t.Fatalf("tree roots=%d err=%v", len(got), err)
	}
	if batches < 6 {
		t.Fatalf("expected bounded base + preload batches, got %d", batches)
	}
	if got[0].ID != root.ID || len(got[0].Permissions) != n || len(got[0].Children) != n {
		t.Fatalf("root=%d permissions=%d children=%d", got[0].ID, len(got[0].Permissions), len(got[0].Children))
	}
	seen := map[uint]bool{}
	for _, p := range got[0].Permissions {
		seen[p.ID] = true
	}
	for _, p := range permissions {
		if !seen[p.ID] {
			t.Fatalf("missing permission %d", p.ID)
		}
	}
	for i, c := range got[0].Children {
		if c.Sort != i+1 || c.ID != children[n-1-i].ID || len(c.Permissions) != 1 || c.Permissions[0].ID != permissions[0].ID {
			t.Fatalf("child %d: %+v", i, c)
		}
	}
	testdb.FailLater(t, db, func() {
		got, err := d.GetMenuTreeContext(ctx, &status)
		if err == nil || got != nil {
			t.Fatalf("partial tree=%d err=%v", len(got), err)
		}
	})
	// A failure in association loading must also suppress the complete base tree.
	const failure = "test:permission_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(failure, func(q *gorm.DB) {
		if q.Statement.Table == "permissions" {
			q.AddError(errors.New("injected permissions failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(failure)
	got, err = d.GetMenuTreeContext(ctx, &status)
	if err == nil || got != nil {
		t.Fatalf("partial permission tree=%d err=%v", len(got), err)
	}
}
