package system

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newCodegenSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:codegen_schema?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	for _, ddl := range []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY,
			tenant_id INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			tenant_id INTEGER NOT NULL DEFAULT 1,
			username TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS project_members (
			id INTEGER PRIMARY KEY,
			project_id INTEGER NOT NULL REFERENCES projects(id),
			user_id INTEGER NOT NULL REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS event_stream_without_pk (
			event_name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS dict_types (
			id INTEGER PRIMARY KEY,
			code TEXT NOT NULL UNIQUE
		)`,
		`INSERT OR IGNORE INTO dict_types (id, code) VALUES (1, 'project_status')`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func TestValidateRequestRejectsUnknownDictionaryType(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	_, err := svc.ValidateRequest(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", DictType: "missing_status", InForm: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing_status") {
		t.Fatalf("unknown dictionary type should be rejected, got %v", err)
	}
}

func TestValidateRequestUsesSelectForExistingDictionaryType(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	validated, err := svc.ValidateRequest(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", DictType: "project_status", InForm: true}},
	})
	if err != nil {
		t.Fatalf("ValidateRequest: %v", err)
	}
	if got := validated.Request.Fields[0].Component; got != ComponentSelect {
		t.Fatalf("component = %q, want %q", got, ComponentSelect)
	}
}

func TestValidateRequestRejectsUnrecognizedManyToManyRelation(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	_, err := svc.ValidateRequest(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", InForm: true}},
		M2Ms: []M2MConfig{{
			Name:         "users",
			JoinTable:    "forged_project_members",
			FKField:      "project_id",
			TargetTable:  "users",
			TargetFK:     "user_id",
			DisplayField: "username",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "forged_project_members") {
		t.Fatalf("unrecognized many-to-many relation should be rejected, got %v", err)
	}
}

func TestValidateRequestAcceptsInspectedManyToManyRelation(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	validated, err := svc.ValidateRequest(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", InForm: true}},
		M2Ms: []M2MConfig{{
			Name:         "users",
			JoinTable:    "project_members",
			FKField:      "project_id",
			TargetTable:  "users",
			TargetFK:     "user_id",
			DisplayField: "username",
		}},
	})
	if err != nil {
		t.Fatalf("ValidateRequest: %v", err)
	}
	if len(validated.Request.M2Ms) != 1 || validated.Request.M2Ms[0].Label != "users" {
		t.Fatalf("normalized m2m = %#v", validated.Request.M2Ms)
	}
}

func TestValidateRequestCarriesSingleMetadataSnapshot(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	validated, err := svc.ValidateRequest(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", InForm: true}},
	})
	if err != nil {
		t.Fatalf("ValidateRequest: %v", err)
	}
	if len(validated.Schemas) < 4 {
		t.Fatalf("metadata snapshot is incomplete: %#v", validated.Schemas)
	}
	if !sort.SliceIsSorted(validated.Schemas, func(i, j int) bool {
		return validated.Schemas[i].Name < validated.Schemas[j].Name
	}) {
		t.Fatalf("metadata snapshot is not sorted: %#v", validated.Schemas)
	}
}

func TestValidateRequestRejectsUnknownSubTableField(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	_, err := svc.ValidateRequest(GenerateRequest{
		Table:   "projects",
		Module:  "projects",
		TplType: TplTypeSub,
		Fields:  []FieldConfig{{Name: "name", InForm: true}},
		Sub: &SubConfig{
			Table:   "project_members",
			FKField: "project_id",
			Fields: []SubFieldConfig{{FieldConfig: FieldConfig{
				Name: "forged_child_field", InForm: true,
			}}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "forged_child_field") {
		t.Fatalf("unknown child field should be rejected, got %v", err)
	}
}

func TestSchemaInspectorLoadsPostgresColumnComments(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.attname AS column_name, COALESCE(col_description(c.oid, a.attnum), '') AS comment`)).
		WithArgs("assets").
		WillReturnRows(sqlmock.NewRows([]string{"column_name", "comment"}).AddRow("name", "资产名称"))

	comments, err := NewSchemaInspector(db).columnComments("assets")
	if err != nil {
		t.Fatalf("columnComments: %v", err)
	}
	if comments["name"] != "资产名称" {
		t.Fatalf("comments = %#v", comments)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListTablesExcludesTablesWithoutPrimaryKeyAndReturnsSummary(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenSchemaDB(t))
	tables, err := svc.ListTables()
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	for _, table := range tables {
		if table.Name == "event_stream_without_pk" {
			t.Fatal("table without primary key must be excluded")
		}
		if table.Name == "projects" {
			if table.PrimaryKey != "id" || table.ColumnCount != 3 || table.RelationCount < 1 {
				t.Fatalf("projects summary = %#v", table)
			}
			return
		}
	}
	t.Fatalf("projects missing from %#v", tables)
}

func TestSchemaInspectorFindsPrimaryKeyAndManyToManyCandidate(t *testing.T) {
	schema, err := NewSchemaInspector(newCodegenSchemaDB(t)).InspectTable("projects")
	if err != nil {
		t.Fatalf("InspectTable: %v", err)
	}
	if schema.PrimaryKey != "id" {
		t.Fatalf("primary key = %q", schema.PrimaryKey)
	}
	if !schema.HasColumn("tenant_id") {
		t.Fatalf("tenant_id missing from %#v", schema.Columns)
	}

	for _, relation := range schema.Relations {
		if relation.Kind == RelationManyToMany &&
			relation.JoinTable == "project_members" &&
			relation.FKField == "project_id" &&
			relation.TargetTable == "users" &&
			relation.TargetFK == "user_id" {
			return
		}
	}
	t.Fatalf("many-to-many candidate missing from %#v", schema.Relations)
}

func TestValidateRequestInfersFormComponents(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenTestDB(t))
	validated, err := svc.ValidateRequest(GenerateRequest{
		Table:  "demo_assets",
		Module: "assets",
		Title:  "资产管理",
		Fields: []FieldConfig{
			{Name: "name", InForm: true},
			{Name: "amount_cents", InForm: true},
			{Name: "active", InForm: true},
		},
	})
	if err != nil {
		t.Fatalf("ValidateRequest: %v", err)
	}
	want := map[string]FormComponent{
		"name": ComponentInput, "amount_cents": ComponentNumber, "active": ComponentSwitch,
	}
	for _, field := range validated.Request.Fields {
		if field.Component != want[field.Name] {
			t.Fatalf("component for %s = %q, want %q", field.Name, field.Component, want[field.Name])
		}
	}
}

func TestValidateRequestRejectsUnknownFormComponent(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenTestDB(t))
	_, err := svc.ValidateRequest(GenerateRequest{
		Table:  "demo_assets",
		Module: "assets",
		Fields: []FieldConfig{{
			Name: "name", InForm: true, Component: FormComponent("script"),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "script") {
		t.Fatalf("unknown form component should be rejected, got %v", err)
	}
}
