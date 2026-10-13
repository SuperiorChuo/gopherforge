package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// 官方 Skills Hub 打通：假 GitHub API 全链测试。

const hubTestCommit = "a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0"

// newHubTestServer 起一个假 GitHub API：/commits/HEAD + /git/trees（recursive）+
// /git/blobs。files: 仓内路径 → 内容；blob sha 用稳定编号映射。
func newHubTestServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	shaByPath := make(map[string]string, len(paths))
	contentBySha := make(map[string]string, len(paths))
	for i, p := range paths {
		sha := fmt.Sprintf("blob%04d", i)
		shaByPath[p] = sha
		contentBySha[sha] = files[p]
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits/HEAD"):
			_ = json.NewEncoder(w).Encode(map[string]string{"sha": hubTestCommit})
		case strings.Contains(r.URL.Path, "/git/trees/"):
			type treeEntry struct {
				Path string `json:"path"`
				Type string `json:"type"`
				SHA  string `json:"sha"`
				Size int64  `json:"size"`
			}
			tree := make([]treeEntry, 0, len(paths))
			for _, p := range paths {
				tree = append(tree, treeEntry{Path: p, Type: "blob", SHA: shaByPath[p], Size: int64(len(files[p]))})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": tree, "truncated": false})
		case strings.Contains(r.URL.Path, "/git/blobs/"):
			sha := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			content, ok := contentBySha[sha]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": base64.StdEncoding.EncodeToString([]byte(content)), "encoding": "base64",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func newHubTestService(t *testing.T, files map[string]string) (*HermesService, *httptest.Server) {
	t.Helper()
	svc, _, _, _ := setupHermesServiceTest(t, true)
	server := newHubTestServer(t, files)
	svc.hubClient = &hermesSkillHubClient{
		apiBase: server.URL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
	return svc, server
}

func TestHubBrowseListsSkillDirs(t *testing.T) {
	files := map[string]string{
		"optional-skills/security/SKILL.md":       "---\nname: security\ndescription: d\n---\nP\n",
		"optional-skills/reporting/SKILL.md":      "---\nname: reporting\ndescription: d\n---\nP\n",
		"optional-skills/reporting/references/r1": "ref",
		"optional-skills/README.md":               "readme",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	browse, err := svc.ListHubSkills(t.Context(), "official", "", "")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if browse.Commit != hubTestCommit {
		t.Fatalf("commit = %q, want pinned head", browse.Commit)
	}
	if len(browse.Entries) != 2 {
		t.Fatalf("entries = %+v, want 2 dirs (README 文件不算)", browse.Entries)
	}
	if browse.Entries[0].Name != "reporting" || browse.Entries[1].Name != "security" {
		t.Fatalf("entries = %+v, want sorted [reporting security]", browse.Entries)
	}
}

func TestHubImportFullChain(t *testing.T) {
	files := map[string]string{
		"optional-skills/weekly-report/SKILL.md":             "---\nname: weekly-report\ndescription: 周报生成\nversion: 1.2.0\n---\nProcedure\n",
		"optional-skills/weekly-report/references/style.md":  "style guide",
		"optional-skills/weekly-report/templates/weekly.tpl": "template",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	skill, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/weekly-report", Ref: hubTestCommit,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if skill.Name != "weekly-report" || skill.Version != "1.2.0" || skill.Status != "pending" {
		t.Fatalf("skill = %+v, want weekly-report@1.2.0 pending", skill)
	}
	if skill.SourceRepo != "NousResearch/hermes-agent" || skill.SourceCommit != hubTestCommit ||
		skill.SourcePath != "optional-skills/weekly-report" {
		t.Fatalf("provenance = %s/%s@%s", skill.SourceRepo, skill.SourcePath, skill.SourceCommit)
	}
	// 官方市场导入无企业签名 → not_applicable
	if skill.SignatureStatus != localmodel.HermesSkillSignatureNotApplicable {
		t.Fatalf("import signature_status = %q, want not_applicable", skill.SignatureStatus)
	}
	if len(skill.SHA256) != 64 {
		t.Fatalf("sha256 = %q", skill.SHA256)
	}
	// 落库内容可再过包校验（tar.gz 合法且含 SKILL.md）
	stored, err := svc.GetSkillContent(t.Context(), skill.ID)
	if err == nil && len(stored) > 0 {
		t.Fatal("pending skill content must not be downloadable (approved-only)")
	}

	// 重复导入同版本 → 友好错误（uk 冲突）
	if _, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/weekly-report", Ref: hubTestCommit,
	}); err == nil {
		t.Fatal("duplicate import must fail")
	}
}

func TestHubImportVersionFallsBackToCommit(t *testing.T) {
	files := map[string]string{
		"optional-skills/no-version/SKILL.md": "---\nname: no-version\ndescription: d\n---\nP\n",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	skill, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/no-version", Ref: hubTestCommit,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if skill.Version != hubTestCommit[:12] {
		t.Fatalf("version = %q, want commit short sha", skill.Version)
	}
}

func TestHubImportRejectsMissingSkillMD(t *testing.T) {
	files := map[string]string{
		"optional-skills/broken/README.md": "no skill md here",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	if _, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/broken", Ref: hubTestCommit,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("missing SKILL.md error = %v, want invalid input", err)
	}
}

func TestHubImportRejectsBadFrontmatter(t *testing.T) {
	files := map[string]string{
		"optional-skills/badfm/SKILL.md": "no frontmatter at all\n",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	if _, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/badfm", Ref: hubTestCommit,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("bad frontmatter error = %v, want invalid input", err)
	}
}

func TestCreateSkillRejectsNonArchiveContent(t *testing.T) {
	// 包校验补课：上传路径也拒非 tar 包/缺 SKILL.md 的内容
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{
		Name: "raw-md", Version: "1.0.0",
	}, []byte("---\nname: raw-md\n---\njust markdown, not a tarball")); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("raw markdown error = %v, want invalid input", err)
	}
	// 合法包过
	if _, err := svc.CreateSkill(ctx, 1, CreateHermesSkillRequest{
		Name: "good-pkg", Version: "1.0.0",
	}, buildHermesTestSkillTarGz(t, "good-pkg", map[string]string{"references/r.md": "ref"})); err != nil {
		t.Fatalf("valid package rejected: %v", err)
	}
}

// 官方 optional-skills 是 <分类>/<技能>/SKILL.md 两层结构（真实仓 111 个技能实证）。
// 回归：browse 必须列出技能目录（security/1password）而非分类目录（security）；
// 导入分类目录必须明确拒绝而不是打出缺 SKILL.md 的空包。
func TestHubBrowseTwoLevelOfficialLayout(t *testing.T) {
	files := map[string]string{
		"optional-skills/security/1password/SKILL.md":      "---\nname: 1password\ndescription: d\n---\nP\n",
		"optional-skills/security/vault/SKILL.md":          "---\nname: vault\ndescription: d\n---\nP\n",
		"optional-skills/creative/blender-mcp/SKILL.md":    "---\nname: blender-mcp\ndescription: d\n---\nP\n",
		"optional-skills/creative/blender-mcp/refs/api.md": "ref",
	}
	svc, server := newHubTestService(t, files)
	defer server.Close()

	browse, err := svc.ListHubSkills(t.Context(), "official", "", "")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(browse.Entries) != 3 {
		t.Fatalf("entries = %+v, want 3 skill dirs (分类目录不该出现)", browse.Entries)
	}
	if browse.Entries[0].Name != "creative/blender-mcp" || browse.Entries[0].Path != "optional-skills/creative/blender-mcp" {
		t.Fatalf("entries[0] = %+v, want creative/blender-mcp", browse.Entries[0])
	}

	// 导入分类目录（无直属 SKILL.md）→ 明确拒绝
	if _, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/security", Ref: hubTestCommit,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("category import error = %v, want invalid input", err)
	}

	// 导入两层技能目录 → 成功
	skill, err := svc.ImportHubSkill(t.Context(), 1, HermesSkillHubImportRequest{
		Source: "official", SkillPath: "optional-skills/security/1password", Ref: hubTestCommit,
	})
	if err != nil {
		t.Fatalf("two-level import: %v", err)
	}
	if skill.Name != "1password" || skill.SourcePath != "optional-skills/security/1password" {
		t.Fatalf("skill = %+v, want 1password from two-level path", skill)
	}
}
