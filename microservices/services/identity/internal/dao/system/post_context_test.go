package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
)

func TestPostDAOGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := newInjectedDepartmentMenuDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewPostDAO(db).GetListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}

func TestPostDAOGetAllUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "sys_posts" ORDER BY sort ASC, created_at ASC`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(3, "dev", "Developer"))

	posts, err := NewPostDAO(db).GetAllContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetAllContext() error = %v", err)
	}
	if len(posts) != 1 || posts[0].Code != "dev" {
		t.Fatalf("GetAllContext() posts = %#v, want one injected row", posts)
	}
}

func TestPostDAOCreateContextUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "sys_posts" ("tenant_id","code","name","sort","status","remark","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING "id"`)).
		WithArgs(uint(1), "dev", "Developer", 5, int8(1), "backend dev", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectCommit()

	post := &model.Post{TenantID: 1, Code: "dev", Name: "Developer", Sort: 5, Status: 1, Remark: "backend dev"}
	if err := NewPostDAO(db).CreateContext(context.Background(), post); err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if post.ID != 9 {
		t.Fatalf("CreateContext() post id = %d, want 9", post.ID)
	}
}

func TestPostDAODeleteContextRejectsWhenUsersAssigned(t *testing.T) {
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "sys_user_posts" WHERE post_id = $1`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(2))

	err := NewPostDAO(db).DeleteContext(context.Background(), 7)
	if !errors.Is(err, ErrPostHasUsers) {
		t.Fatalf("DeleteContext() error = %v, want ErrPostHasUsers", err)
	}
}

func TestPostDAODeleteContextDeletesWhenUnassigned(t *testing.T) {
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "sys_user_posts" WHERE post_id = $1`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "sys_posts" WHERE "sys_posts"."id" = $1`)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewPostDAO(db).DeleteContext(context.Background(), 7); err != nil {
		t.Fatalf("DeleteContext() error = %v", err)
	}
}

func TestUserDAOAssignPostsContextReplacesAssignments(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "sys_user_posts" WHERE user_id = $1`)).
		WithArgs(uint(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "sys_user_posts" ("user_id","post_id") VALUES ($1,$2),($3,$4) RETURNING "id"`)).
		WithArgs(uint(5), uint(1), uint(5), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11).AddRow(12))
	mock.ExpectCommit()

	if err := NewUserDAO(db).AssignPostsContext(context.Background(), 5, []uint{1, 2}); err != nil {
		t.Fatalf("AssignPostsContext() error = %v", err)
	}
}

func TestUserDAOAssertPostsInTenantContextRejectsForeignPosts(t *testing.T) {
	db, mock := setupSystemDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "sys_posts" WHERE tenant_id = $1 AND id IN ($2,$3)`)).
		WithArgs(uint(1), uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))

	err := NewUserDAO(db).AssertPostsInTenantContext(context.Background(), []uint{1, 2}, 1)
	if !errors.Is(err, ErrPostNotInTenant) {
		t.Fatalf("AssertPostsInTenantContext() error = %v, want ErrPostNotInTenant", err)
	}
}
