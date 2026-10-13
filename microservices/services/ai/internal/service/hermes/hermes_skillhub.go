package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/goccy/go-yaml"
)

// hermesSkillHubRepoPattern 约束 custom 源的 owner/repo 形态。
var hermesSkillHubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}/[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// Hermes 技能市场打通官方 Skills Hub（GitHub 系源）。
//
// 控制面服务端从 GitHub 抓取技能目录（agentskills.io 标准：目录 + SKILL.md），
// 内存打包成我方 tarball 格式，走既有 CreateSkill 落 pending——审批 gate、
// 员工自装、manager 分发链路零改动。不调用上游 `hermes skills install`（端侧 CLI）。
//
// 安全姿态：出网仅允许配置内 GitHub host；导入技能与手工上传同等地位（pending
// 待审批）；上游 skills_guard 扫描被绕过，我方审批是唯一 gate（文档已注明）。

const (
	// hermesSkillHubMaxFiles 限制单技能文件数，防误导入巨型目录。
	hermesSkillHubMaxFiles = 200
	// hermesSkillHubMaxDepth 限制递归深度（agentskills.io 约定引用一层为宜）。
	hermesSkillHubMaxDepth = 4
)

// HermesSkillHubSource 是内建源：GitHub 仓 + 技能目录前缀。
type HermesSkillHubSource struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Repo string `json:"repo"`
	Path string `json:"path"`
}

// hermesSkillHubSources 是 v1 内建源注册表。官方源 = Hermes 仓 optional-skills/；
// 四个受信 taps 与上游 Skills Hub 默认 taps 一致（目录布局按各仓实际，root 兜底，
// 不合布局时管理员可用 custom 指定 path）。
var hermesSkillHubSources = []HermesSkillHubSource{
	{Key: "official", Name: "Hermes 官方（optional-skills）", Repo: "NousResearch/hermes-agent", Path: "optional-skills"},
	{Key: "openai", Name: "OpenAI skills", Repo: "openai/skills", Path: ""},
	{Key: "anthropics", Name: "Anthropic skills", Repo: "anthropics/skills", Path: ""},
	{Key: "huggingface", Name: "HuggingFace skills", Repo: "huggingface/skills", Path: ""},
	{Key: "nvidia", Name: "NVIDIA skills", Repo: "NVIDIA/skills", Path: ""},
}

func resolveHermesSkillHubSource(source, customRepo, customPath string) (repo, prefix string, err error) {
	if source == "custom" {
		repo = strings.TrimSpace(customRepo)
		if !hermesSkillHubRepoPattern.MatchString(repo) {
			return "", "", fmt.Errorf("%w: invalid custom repo", ErrHermesInvalidInput)
		}
		return repo, strings.Trim(strings.TrimSpace(customPath), "/"), nil
	}
	for _, s := range hermesSkillHubSources {
		if s.Key == source {
			return s.Repo, s.Path, nil
		}
	}
	return "", "", fmt.Errorf("%w: unknown hub source", ErrHermesInvalidInput)
}

// hermesSkillHubClient 是固定 host 的 GitHub 客户端。base 可注入（测试/镜像）。
type hermesSkillHubClient struct {
	apiBase string
	token   string
	client  *http.Client
}

func newHermesSkillHubClient() *hermesSkillHubClient {
	apiBase := strings.TrimRight(os.Getenv("HERMES_SKILLHUB_GITHUB_API"), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &hermesSkillHubClient{
		apiBase: apiBase,
		token:   strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				// 只允许跳到 https 的 github 域（api/raw/codeload/objects CDN）
				if req.URL.Scheme != "https" || !isGitHubHost(req.URL.Host) {
					return fmt.Errorf("redirect to disallowed host %s", req.URL.Host)
				}
				return nil
			},
		},
	}
}

func isGitHubHost(host string) bool {
	host = strings.ToLower(host)
	return host == "api.github.com" || host == "raw.githubusercontent.com" ||
		host == "codeload.github.com" || strings.HasSuffix(host, ".githubusercontent.com") ||
		host == "github.com"
}

func (c *hermesSkillHubClient) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid hub url", ErrHermesInvalidInput)
	}
	// 同注入 base 的 host：scheme 随 base（测试 httptest 为 http、生产镜像为 https）；
	// 跨 host（如 contents API 的 download_url）：必须 https 且是 GitHub 域。
	if baseURL, baseErr := url.Parse(c.apiBase); baseErr != nil || parsed.Host != baseURL.Host {
		if parsed.Scheme != "https" || !isGitHubHost(parsed.Host) {
			return nil, fmt.Errorf("%w: hub url host not allowed", ErrHermesInvalidInput)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "go-admin-kit-hermes-skillhub")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hub fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: hub path not found", ErrHermesInvalidInput)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: hub rate limited (configure GITHUB_TOKEN)", ErrHermesInvalidInput)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub fetch status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: hub response too large", ErrHermesInvalidInput)
	}
	return data, nil
}

// hubTreeEntry 是 git trees API 的一项（blob=文件，tree=目录）。
type hubTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int64  `json:"size"`
}

// getTree 一次拉取整仓文件树（recursive）。官方 optional-skills 是
// <分类>/<技能>/SKILL.md 两层结构，逐层 contents 列目录会把分类目录误当技能
// （曾致导入报「缺 SKILL.md」）；整树按 SKILL.md 定位技能目录天然兼容任意层级。
func (c *hermesSkillHubClient) getTree(ctx context.Context, repo, ref string) ([]hubTreeEntry, error) {
	u := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", c.apiBase, repo, url.PathEscape(ref))
	data, err := c.get(ctx, u, 16<<20)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tree      []hubTreeEntry `json:"tree"`
		Truncated bool           `json:"truncated"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("hub tree decode failed: %w", err)
	}
	if out.Truncated {
		return nil, fmt.Errorf("%w: repo tree too large, use a custom source with a narrower path", ErrHermesInvalidInput)
	}
	return out.Tree, nil
}

// getBlob 按 sha 取文件字节（blobs API，base64；不受 contents API 1MB 限制）。
func (c *hermesSkillHubClient) getBlob(ctx context.Context, repo, sha string, limit int64) ([]byte, error) {
	u := fmt.Sprintf("%s/repos/%s/git/blobs/%s", c.apiBase, repo, url.PathEscape(sha))
	data, err := c.get(ctx, u, limit*2) // base64 膨胀 ~4/3，留余量
	if err != nil {
		return nil, err
	}
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("hub blob decode failed: %w", err)
	}
	if out.Encoding != "base64" {
		return nil, fmt.Errorf("%w: unexpected hub blob encoding", ErrHermesInvalidInput)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("hub blob base64 decode failed: %w", err)
	}
	if int64(len(decoded)) > limit {
		return nil, fmt.Errorf("%w: hub file too large", ErrHermesInvalidInput)
	}
	return decoded, nil
}

// resolveHeadCommit 解析仓库默认分支 HEAD commit sha（导入钉此不可变引用）。
func (c *hermesSkillHubClient) resolveHeadCommit(ctx context.Context, repo string) (string, error) {
	data, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/commits/HEAD", c.apiBase, repo), 1<<20)
	if err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(data, &commit); err != nil || len(commit.SHA) < 7 {
		return "", fmt.Errorf("hub head commit decode failed")
	}
	return commit.SHA, nil
}

// ---------------------------------------------------------------------------
// Browse / Import
// ---------------------------------------------------------------------------

// HermesSkillHubEntry 是市场浏览条目（一个候选技能目录）。
type HermesSkillHubEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// HermesSkillHubBrowse 是浏览响应：源信息 + 钉住的 commit + 目录列表。
type HermesSkillHubBrowse struct {
	Repo    string                 `json:"repo"`
	Path    string                 `json:"path"`
	Commit  string                 `json:"commit"`
	Entries []HermesSkillHubEntry  `json:"entries"`
	Sources []HermesSkillHubSource `json:"sources"`
}

// ListHubSkills 浏览官方市场：整树定位「含 SKILL.md 的目录」为技能（兼容官方
// optional-skills 的 分类/技能 两层结构与 taps 的平铺结构），钉住当前 HEAD commit。
func (s *HermesService) ListHubSkills(ctx context.Context, source, customRepo, customPath string) (*HermesSkillHubBrowse, error) {
	repo, prefix, err := resolveHermesSkillHubSource(source, customRepo, customPath)
	if err != nil {
		return nil, err
	}
	client := s.skillHubClient()
	commit, err := client.resolveHeadCommit(ctx, repo)
	if err != nil {
		return nil, err
	}
	tree, err := client.getTree(ctx, repo, commit)
	if err != nil {
		return nil, err
	}
	out := make([]HermesSkillHubEntry, 0)
	for _, entry := range tree {
		if entry.Type != "blob" || !strings.HasSuffix(entry.Path, "/SKILL.md") {
			continue
		}
		dir := strings.TrimSuffix(entry.Path, "/SKILL.md")
		if prefix != "" && dir != prefix && !strings.HasPrefix(dir, prefix+"/") {
			continue
		}
		if strings.HasPrefix(path.Base(dir), ".") || strings.Contains(dir, "/.") {
			continue // 隐藏目录不进目录
		}
		name := dir
		if prefix != "" && dir != prefix {
			name = strings.TrimPrefix(dir, prefix+"/")
		}
		out = append(out, HermesSkillHubEntry{Name: name, Path: dir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return &HermesSkillHubBrowse{
		Repo: repo, Path: prefix, Commit: commit,
		Entries: out, Sources: hermesSkillHubSources,
	}, nil
}

// skillHubClient 允许测试注入（默认按 env 构造）。
func (s *HermesService) skillHubClient() *hermesSkillHubClient {
	if s.hubClient != nil {
		return s.hubClient
	}
	return newHermesSkillHubClient()
}

// HermesSkillHubImportRequest 是导入请求。Ref 用 browse 返回的 commit（不可变）。
type HermesSkillHubImportRequest struct {
	Source     string `json:"source"`
	CustomRepo string `json:"custom_repo"`
	CustomPath string `json:"custom_path"`
	SkillPath  string `json:"skill_path"` // 技能目录相对仓根路径
	Ref        string `json:"ref"`        // commit sha；空则解析 HEAD
}

// ImportHubSkill 从官方市场导入技能：抓目录文件 → 内存打 tar.gz → 解析 SKILL.md
// frontmatter → 走 CreateSkill 落 pending + provenance。
func (s *HermesService) ImportHubSkill(ctx context.Context, actorID uint, request HermesSkillHubImportRequest) (*localmodel.HermesSkill, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	repo, _, err := resolveHermesSkillHubSource(request.Source, request.CustomRepo, request.CustomPath)
	if err != nil {
		return nil, err
	}
	skillPath := strings.Trim(strings.TrimSpace(request.SkillPath), "/")
	if skillPath == "" || strings.Contains(skillPath, "..") {
		return nil, fmt.Errorf("%w: invalid skill path", ErrHermesInvalidInput)
	}
	client := s.skillHubClient()
	ref := strings.TrimSpace(request.Ref)
	if ref == "" {
		if ref, err = client.resolveHeadCommit(ctx, repo); err != nil {
			return nil, err
		}
	}
	if !hermesCommitPattern.MatchString(ref) {
		return nil, fmt.Errorf("%w: ref must be a commit sha", ErrHermesInvalidInput)
	}

	// 整树定位技能目录文件（数量/总大小有界）。要求 skillPath 直接含 SKILL.md：
	// 分类目录（如官方 optional-skills/<分类>）会被明确拒绝而不是打出空包。
	files, err := s.collectHubFiles(ctx, client, repo, skillPath, ref)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: skill directory is empty", ErrHermesInvalidInput)
	}

	skillName := strings.ToLower(path.Base(skillPath))
	content, err := buildSkillTarGz(skillName, skillPath, files)
	if err != nil {
		return nil, err
	}

	// 解析 SKILL.md frontmatter 提取展示元数据与版本
	meta, err := parseSkillArchiveMeta(content)
	if err != nil {
		return nil, err
	}
	version := meta.Version
	if version == "" {
		version = ref[:12] // 无版本则钉 commit 短 sha（不可变、过 version 正则）
	}
	displayName := meta.Name
	if displayName == "" {
		displayName = skillName
	}

	skill, err := s.CreateSkill(ctx, actorID, CreateHermesSkillRequest{
		Name:        skillName,
		DisplayName: displayName,
		Description: meta.Description,
		Version:     version,
	}, content)
	if err != nil {
		return nil, err
	}
	// 补 provenance 三列 + 签名状态（CreateSkill 不感知来源，导入路径单独回写）：
	// 官方市场无企业签名，标记 not_applicable（靠 repo@commit 来源锚定）。
	if err := s.dao.UpdateSkillSource(ctx, skill.ID, repo, skillPath, ref); err != nil {
		return nil, err
	}
	if err := s.dao.UpdateSkillSignatureStatus(ctx, skill.ID, localmodel.HermesSkillSignatureNotApplicable); err != nil {
		return nil, err
	}
	skill.SourceRepo, skill.SourcePath, skill.SourceCommit = repo, skillPath, ref
	skill.SignatureStatus = localmodel.HermesSkillSignatureNotApplicable
	return skill, nil
}

type hubFile struct {
	relPath string // 相对技能目录
	data    []byte
}

func (s *HermesService) collectHubFiles(ctx context.Context, client *hermesSkillHubClient, repo, skillPath, ref string) ([]hubFile, error) {
	tree, err := client.getTree(ctx, repo, ref)
	if err != nil {
		return nil, err
	}
	prefix := skillPath + "/"
	blobs := make([]hubTreeEntry, 0)
	hasSkillMD := false
	var total int64
	for _, entry := range tree {
		if entry.Type != "blob" || !strings.HasPrefix(entry.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(entry.Path, prefix)
		if rel == "SKILL.md" {
			hasSkillMD = true
		}
		if strings.Count(rel, "/") >= hermesSkillHubMaxDepth {
			return nil, fmt.Errorf("%w: skill directory too deep", ErrHermesInvalidInput)
		}
		blobs = append(blobs, entry)
		total += entry.Size
		if len(blobs) > hermesSkillHubMaxFiles {
			return nil, fmt.Errorf("%w: skill has too many files", ErrHermesInvalidInput)
		}
		if total > int64(maxHermesSkillContentBytes) {
			return nil, fmt.Errorf("%w: skill exceeds size limit", ErrHermesInvalidInput)
		}
	}
	if !hasSkillMD {
		return nil, fmt.Errorf("%w: skill directory does not contain SKILL.md (did you pick a category directory?)", ErrHermesInvalidInput)
	}
	files := make([]hubFile, 0, len(blobs))
	for _, blob := range blobs {
		data, err := client.getBlob(ctx, repo, blob.SHA, int64(maxHermesSkillContentBytes))
		if err != nil {
			return nil, err
		}
		files = append(files, hubFile{relPath: strings.TrimPrefix(blob.Path, prefix), data: data})
	}
	return files, nil
}

// buildSkillTarGz 把文件集打成 `<name>/...` 顶层目录的 tar.gz（stripTop 语义兼容）。
func buildSkillTarGz(skillName, skillPath string, files []hubFile) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// 排序保证同一来源重复导入字节一致（sha256 稳定）
	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })
	for _, f := range files {
		if strings.Contains(f.relPath, "..") {
			return nil, fmt.Errorf("%w: unsafe file path in skill", ErrHermesInvalidInput)
		}
		hdr := &tar.Header{
			Name: path.Join(skillName, f.relPath),
			Mode: 0o644,
			Size: int64(len(f.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > maxHermesSkillContentBytes {
		return nil, fmt.Errorf("%w: skill exceeds size limit", ErrHermesInvalidInput)
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// 包格式校验（上传与导入共用）
// ---------------------------------------------------------------------------

// hermesSkillMeta 是 SKILL.md frontmatter 的解析结果（agentskills.io 标准字段子集）。
type hermesSkillMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Metadata    struct {
		Version string `yaml:"version"`
	} `yaml:"metadata"`
}

// parseSkillArchiveMeta 校验技能包格式并解析 SKILL.md frontmatter：
// tar(.gz) 可解、SKILL.md 存在（顶层或单一顶层目录下，与 manager stripTop 语义一致）、
// frontmatter 可解析且 name/description 非空。返回展示元数据。
func parseSkillArchiveMeta(content []byte) (*hermesSkillMeta, error) {
	var reader io.Reader = bytes.NewReader(content)
	if gz, err := gzip.NewReader(bytes.NewReader(content)); err == nil {
		defer gz.Close()
		reader = gz
	}
	tr := tar.NewReader(reader)
	var skillMD []byte
	var sawEntry bool
	const maxUncompressed = 64 << 20 // 解压炸弹防护
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: skill package is not a valid tar archive", ErrHermesInvalidInput)
		}
		sawEntry = true
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		total += hdr.Size
		if total > maxUncompressed {
			return nil, fmt.Errorf("%w: skill package uncompressed size too large", ErrHermesInvalidInput)
		}
		clean := strings.TrimPrefix(path.Clean(hdr.Name), "./")
		// SKILL.md 在顶层，或单一顶层目录下（stripTop 语义）
		parts := strings.Split(clean, "/")
		isSkillMD := clean == "SKILL.md" || (len(parts) == 2 && parts[1] == "SKILL.md")
		if isSkillMD && skillMD == nil {
			data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, fmt.Errorf("%w: read SKILL.md failed", ErrHermesInvalidInput)
			}
			skillMD = data
		}
	}
	if !sawEntry {
		return nil, fmt.Errorf("%w: skill package is empty", ErrHermesInvalidInput)
	}
	if skillMD == nil {
		return nil, fmt.Errorf("%w: skill package must contain SKILL.md", ErrHermesInvalidInput)
	}
	meta, err := parseSkillFrontmatter(skillMD)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func parseSkillFrontmatter(md []byte) (*hermesSkillMeta, error) {
	text := string(md)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("%w: SKILL.md missing YAML frontmatter", ErrHermesInvalidInput)
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("%w: SKILL.md frontmatter not terminated", ErrHermesInvalidInput)
	}
	var meta hermesSkillMeta
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return nil, fmt.Errorf("%w: SKILL.md frontmatter is invalid yaml", ErrHermesInvalidInput)
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Version == "" {
		meta.Version = strings.TrimSpace(meta.Metadata.Version)
	}
	if meta.Name == "" || meta.Description == "" {
		return nil, fmt.Errorf("%w: SKILL.md frontmatter requires name and description", ErrHermesInvalidInput)
	}
	return &meta, nil
}
