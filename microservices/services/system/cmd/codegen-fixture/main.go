package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/glebarez/sqlite"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"gorm.io/gorm"
)

type fixtureResult struct {
	Digest string   `json:"digest"`
	Paths  []string `json:"paths"`
}

func main() {
	repositoryRoot := flag.String("repo", "", "临时仓库根目录")
	flag.Parse()
	if *repositoryRoot == "" {
		exitError(fmt.Errorf("必须提供 --repo"))
	}

	db, err := newFixtureDB()
	if err != nil {
		exitError(err)
	}
	writer, err := systemsvc.NewRepositoryWriter(*repositoryRoot)
	if err != nil {
		exitError(err)
	}
	request := systemsvc.GenerateRequest{
		Table:  "codegen_fixture_records",
		Module: "codegenfixture",
		Title:  "代码生成契约",
		Fields: []systemsvc.FieldConfig{
			{Name: "name", Label: "名称", InList: true, InSearch: true, InForm: true, Required: true},
			{Name: "status", Label: "状态", InList: true, InSearch: true, InForm: true},
		},
	}
	plan, err := systemsvc.NewCodegenServiceWithDB(db).BuildPlan(request, writer)
	if err != nil {
		exitError(err)
	}
	written, err := writer.Write(plan)
	if err != nil {
		exitError(err)
	}
	paths := append(append([]string{}, written.Created...), written.Patched...)
	sort.Strings(paths)
	if err := json.NewEncoder(os.Stdout).Encode(fixtureResult{Digest: written.Digest, Paths: paths}); err != nil {
		exitError(err)
	}
}

func newFixtureDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("创建 SQLite 夹具失败: %w", err)
	}
	const schema = `CREATE TABLE codegen_fixture_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(120) NOT NULL,
		status INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME,
		updated_at DATETIME
	)`
	if err := db.Exec(schema).Error; err != nil {
		return nil, fmt.Errorf("创建代码生成夹具表失败: %w", err)
	}
	return db, nil
}

func exitError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
