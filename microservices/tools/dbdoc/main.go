// dbdoc：连接 PostgreSQL 读取 pg_catalog/information_schema，按域分组生成数据库结构 Markdown 文档。
// 连活库（而非解析迁移 SQL）以保证拿到的是全量真实 schema（含 AutoMigrate 维护的列与索引）。
//
// 用法：
//	DB_HOST=... DB_PORT=... DB_USER=... DB_PASSWORD=... DB_NAME=... go run . --out ../../../docs/database-schema.md
//	go run . --public --lang en --out database-public.en.md   # 只输出基础设施域（供公开脚手架站）
//
// 连接参数取 env（与各服务同名）：DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME/DB_SSLMODE，或用 --dsn 整串覆盖。
// 输出确定有序（域/表/列/索引全部稳定排序），同一 schema 重复生成零 diff，适合进 git 做漂移审查。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type column struct {
	Name    string
	Type    string
	NotNull bool
	Default string
	Comment string
}

type table struct {
	Name        string
	Comment     string
	Columns     []column
	Constraints []string // pg_get_constraintdef 全文（PK/UNIQUE/FK/CHECK）
	Indexes     []string // 非约束索引的 indexdef
}

// domain 定义一个分组；Public 标记该域是否属于基础设施（可进公开脚手架文档）。
type domain struct {
	Key      string
	ZH, EN   string
	Public   bool
	Prefixes []string // 前缀或全名匹配，按声明顺序优先
}

// 域清单覆盖脚手架全部基础表（auth/identity/system/audit/file + tenant/bpm）。
// 二次开发新增的业务表落入 unclassified，且**默认不进 --public 输出**（default-deny，防业务表泄漏）；
// 按需在此追加自己的业务域分组。
var domains = []domain{
	{Key: "identity", ZH: "身份与权限", EN: "Identity & RBAC", Public: true, Prefixes: []string{
		"users", "roles", "permissions", "menus", "menu_permissions", "role_permissions",
		"user_roles", "role_data_scope_departments", "departments", "sys_posts", "sys_user_posts",
	}},
	{Key: "auth", ZH: "认证与安全", EN: "Auth & Security", Public: true, Prefixes: []string{
		"oauth_bindings", "password_history", "totp_recovery_codes", "console_sessions", "console_routes",
	}},
	{Key: "tenant", ZH: "多租户", EN: "Multi-tenancy", Public: true, Prefixes: []string{
		"tenants", "tenant_packages",
	}},
	{Key: "system", ZH: "系统运营", EN: "System Ops", Public: true, Prefixes: []string{
		"dict_", "notices", "error_codes", "system_settings", "sms_", "mail_",
		"scheduled_", "codegen_",
	}},
	{Key: "audit", ZH: "审计日志", EN: "Audit Logs", Public: true, Prefixes: []string{
		"login_logs", "operation_logs", "audit_logs",
	}},
	{Key: "file", ZH: "文件服务", EN: "File Service", Public: true, Prefixes: []string{
		"files", "file_",
	}},
	{Key: "bpm", ZH: "审批流（BPM）", EN: "Workflow (BPM)", Public: true, Prefixes: []string{
		"bpm_",
	}},
	{Key: "meta", ZH: "迁移框架", EN: "Migration Meta", Public: true, Prefixes: []string{
		"schema_migrations", "goose_db_version",
	}},
}

var unclassified = domain{Key: "unclassified", ZH: "未分类（请补充 dbdoc 域清单）", EN: "Unclassified (extend dbdoc domain list)", Public: false}

func classify(name string) domain {
	for _, d := range domains {
		for _, p := range d.Prefixes {
			if name == p || (strings.HasSuffix(p, "_") && strings.HasPrefix(name, p)) {
				return d
			}
		}
	}
	return unclassified
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func buildDSN() string {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s",
		env("DB_HOST", "127.0.0.1"), env("DB_PORT", "5432"),
		env("DB_USER", "postgres"), env("DB_NAME", "go_admin_kit"), env("DB_SSLMODE", "disable"))
	if pw := os.Getenv("DB_PASSWORD"); pw != "" {
		dsn += " password=" + pw
	}
	return dsn
}

func loadTables(ctx context.Context, conn *pgx.Conn) ([]table, error) {
	rows, err := conn.Query(ctx, `
		SELECT c.relname, COALESCE(obj_description(c.oid, 'pg_class'), '')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	var tables []table
	for rows.Next() {
		var t table
		if err := rows.Scan(&t.Name, &t.Comment); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range tables {
		if err := loadTableDetail(ctx, conn, &tables[i]); err != nil {
			return nil, fmt.Errorf("表 %s: %w", tables[i].Name, err)
		}
	}
	return tables, nil
}

func loadTableDetail(ctx context.Context, conn *pgx.Conn, t *table) error {
	rows, err := conn.Query(ctx, `
		SELECT a.attname,
		       format_type(a.atttypid, a.atttypmod),
		       a.attnotnull,
		       CASE WHEN a.attidentity <> '' THEN 'IDENTITY'
		            ELSE COALESCE(pg_get_expr(d.adbin, d.adrelid), '') END,
		       COALESCE(col_description(a.attrelid, a.attnum), '')
		FROM pg_attribute a
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, pgx.Identifier{"public", t.Name}.Sanitize())
	if err != nil {
		return err
	}
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &c.Default, &c.Comment); err != nil {
			return err
		}
		t.Columns = append(t.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	conNames := map[string]bool{}
	rows, err = conn.Query(ctx, `
		SELECT conname, pg_get_constraintdef(oid)
		FROM pg_constraint WHERE conrelid = $1::regclass AND contype IN ('p','u','f')
		ORDER BY contype, conname`, pgx.Identifier{"public", t.Name}.Sanitize())
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return err
		}
		conNames[name] = true
		t.Constraints = append(t.Constraints, def)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = conn.Query(ctx, `
		SELECT indexname, indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = $1 ORDER BY indexname`, t.Name)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return err
		}
		if conNames[name] {
			continue // PK/UNIQUE 约束隐含的索引不重复列出
		}
		// 只保留列部分，去掉 "CREATE [UNIQUE] INDEX ... ON public.xxx USING" 前缀噪声
		t.Indexes = append(t.Indexes, strings.TrimSpace(def[strings.Index(def, "USING")+5:])+uniqMark(def))
	}
	return rows.Err()
}

func uniqMark(indexdef string) string {
	if strings.HasPrefix(indexdef, "CREATE UNIQUE") {
		return " [UNIQUE]"
	}
	return ""
}

type i18n struct{ Title, Intro, Domain, Table, Col, Type, Nullable, Default, Comment, Keys, Indexes, Yes, No, TOC string }

var texts = map[string]i18n{
	"zh": {
		Title: "数据库表结构", Intro: "本文档由 `tools/dbdoc` 连接数据库自动生成，请勿手改；schema 变更后重新生成。",
		Domain: "域", Table: "表", Col: "列", Type: "类型", Nullable: "可空", Default: "默认值", Comment: "说明",
		Keys: "键与约束", Indexes: "索引", Yes: "是", No: "否", TOC: "目录",
	},
	"en": {
		Title: "Database Schema", Intro: "Auto-generated by `tools/dbdoc` from a live database. Do not edit by hand; regenerate after schema changes.",
		Domain: "Domain", Table: "Table", Col: "Column", Type: "Type", Nullable: "Nullable", Default: "Default", Comment: "Comment",
		Keys: "Keys & Constraints", Indexes: "Indexes", Yes: "yes", No: "no", TOC: "Contents",
	},
}

func render(tables []table, lang string, public bool) string {
	tr := texts[lang]
	name := func(d domain) string {
		if lang == "en" {
			return d.EN
		}
		return d.ZH
	}

	grouped := map[string][]table{}
	order := append(append([]domain{}, domains...), unclassified)
	for _, t := range tables {
		d := classify(t.Name)
		if public && !d.Public {
			continue
		}
		grouped[d.Key] = append(grouped[d.Key], t)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", tr.Title, tr.Intro)

	fmt.Fprintf(&b, "## %s\n\n", tr.TOC)
	for _, d := range order {
		ts := grouped[d.Key]
		if len(ts) == 0 {
			continue
		}
		names := make([]string, len(ts))
		for i, t := range ts {
			names[i] = fmt.Sprintf("`%s`", t.Name)
		}
		fmt.Fprintf(&b, "- **%s**（%d）：%s\n", name(d), len(ts), strings.Join(names, " "))
	}
	b.WriteString("\n")

	for _, d := range order {
		ts := grouped[d.Key]
		if len(ts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", name(d))
		for _, t := range ts {
			fmt.Fprintf(&b, "### %s\n\n", t.Name)
			if t.Comment != "" {
				fmt.Fprintf(&b, "%s\n\n", t.Comment)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n|---|---|---|---|---|\n", tr.Col, tr.Type, tr.Nullable, tr.Default, tr.Comment)
			for _, c := range t.Columns {
				nullable := tr.Yes
				if c.NotNull {
					nullable = tr.No
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", c.Name, c.Type, nullable, mdEscape(c.Default), mdEscape(c.Comment))
			}
			b.WriteString("\n")
			if len(t.Constraints) > 0 {
				fmt.Fprintf(&b, "**%s**\n\n", tr.Keys)
				for _, c := range t.Constraints {
					fmt.Fprintf(&b, "- `%s`\n", c)
				}
				b.WriteString("\n")
			}
			if len(t.Indexes) > 0 {
				fmt.Fprintf(&b, "**%s**\n\n", tr.Indexes)
				for _, idx := range t.Indexes {
					fmt.Fprintf(&b, "- `%s`\n", idx)
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func mdEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func main() {
	dsn := flag.String("dsn", "", "libpq DSN（默认由 DB_* env 拼装）")
	out := flag.String("out", "", "输出文件（默认 stdout）")
	lang := flag.String("lang", "zh", "文档语言 zh|en")
	public := flag.Bool("public", false, "只输出基础设施域（公开脚手架用，业务表默认拒绝）")
	flag.Parse()

	if _, ok := texts[*lang]; !ok {
		fmt.Fprintln(os.Stderr, "--lang 只支持 zh|en")
		os.Exit(2)
	}
	connStr := *dsn
	if connStr == "" {
		connStr = buildDSN()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "连接数据库失败：", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	tables, err := loadTables(ctx, conn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 schema 失败：", err)
		os.Exit(1)
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })

	doc := render(tables, *lang, *public)
	if *out == "" {
		fmt.Print(doc)
		return
	}
	if err := os.WriteFile(*out, []byte(doc), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写文件失败：", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "已生成 %s（%d 张表，lang=%s public=%v）\n", *out, len(tables), *lang, *public)
}
