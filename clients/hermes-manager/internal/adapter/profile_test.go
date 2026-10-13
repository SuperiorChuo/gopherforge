package adapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

func testDesired(revision uint64) *protocol.DesiredState {
	return &protocol.DesiredState{
		UnsignedDesiredState: protocol.UnsignedDesiredState{
			SchemaVersion:  1,
			Revision:       revision,
			DesiredState:   "active",
			LeaseExpiresAt: time.Now().Add(time.Hour),
			Profile:        "company",
			Model: protocol.DesiredStateModel{
				Mode:    "company_proxy",
				BaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
				Model:   "approved-model",
			},
			// 刻意乱序：渲染必须排序后输出，否则 digest 会随入参顺序抖动
			Toolsets: []string{"todo", "clarify", "no_mcp", "memory"},
			Privacy:  protocol.Privacy{},
		},
	}
}

func newProfileTestBase(t *testing.T) *Base {
	t.Helper()
	base, err := newBase(Options{Home: t.TempDir(), Runner: &fakeRunner{}}, &fakeProcess{})
	if err != nil {
		t.Fatalf("newBase: %v", err)
	}
	return base
}

// 受管字段必须落到上游认得的键上，否则 Hermes 会忽略企业策略照跑默认配置。
func TestApplyProfileWritesUpstreamKeys(t *testing.T) {
	base := newProfileTestBase(t)
	ctx := context.Background()

	digest, changed, err := base.ApplyProfile(ctx, testDesired(7), "token-abc")
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if digest == "" {
		t.Fatal("digest is empty")
	}
	if changed {
		t.Fatal("首次渲染不该报 changed（没有旧内容就谈不上漂移）")
	}

	config, err := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	rendered := string(config)
	for _, want := range []string{
		`_config_version: 33`, // 缺它会被判 v0，doctor 劝员工跑 --fix 重写整个受管文件
		`provider: "gak-company"`,
		`  gak-company:`,
		`    base_url: "https://control.example/api/v1/ai/hermes/proxy/v1"`,
		`    key_env: "GAK_HERMES_MODEL_TOKEN"`,
		`default: "approved-model"`,
		`cli: ["clarify", "memory", "no_mcp", "todo"]`, // 已排序
		"hard_stop_enabled: true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("config.yaml 缺少 %q，实际内容：\n%s", want, rendered)
		}
	}

	env, err := os.ReadFile(filepath.Join(base.profileDir(), ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(env), "GAK_HERMES_MODEL_TOKEN=token-abc") {
		t.Fatalf(".env 未写入受管 token：%s", env)
	}
	// 凭据绝不能落进 config.yaml：那会让 digest 随每次 token 轮换变动，
	// 把设计内的轮换伪装成配置漂移。
	if strings.Contains(rendered, "token-abc") {
		t.Fatalf("凭据泄漏进 config.yaml：\n%s", rendered)
	}
}

// 幂等性是漂移检测的基础：同一策略反复渲染必须字节一致、digest 不变、changed=false。
func TestApplyProfileIsIdempotent(t *testing.T) {
	base := newProfileTestBase(t)
	ctx := context.Background()

	first, _, err := base.ApplyProfile(ctx, testDesired(7), "token-1")
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, changed, err := base.ApplyProfile(ctx, testDesired(7), "token-2")
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if first != second {
		t.Fatalf("digest 不稳定：%s != %s", first, second)
	}
	if changed {
		t.Fatal("同一策略重复渲染不该报 changed")
	}
}

// 员工手工改了受管 config → 必须被检测为漂移并覆盖回策略内容。
func TestApplyProfileDetectsDriftAndRestores(t *testing.T) {
	base := newProfileTestBase(t)
	ctx := context.Background()
	if _, _, err := base.ApplyProfile(ctx, testDesired(7), "token-1"); err != nil {
		t.Fatalf("seed apply: %v", err)
	}

	tampered := "model:\n  provider: \"anthropic\"\n"
	if err := os.WriteFile(filepath.Join(base.profileDir(), "config.yaml"), []byte(tampered), 0o600); err != nil {
		t.Fatalf("tamper config: %v", err)
	}

	_, changed, err := base.ApplyProfile(ctx, testDesired(7), "token-1")
	if err != nil {
		t.Fatalf("apply after tamper: %v", err)
	}
	if !changed {
		t.Fatal("手工篡改后必须报 changed（服务端据此记录 policy_drift）")
	}
	restored, err := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if strings.Contains(string(restored), "anthropic") {
		t.Fatalf("篡改内容未被覆盖：\n%s", restored)
	}
}

// token 轮换是设计内行为，不能被当成配置漂移上报，也不能进 digest。
func TestTokenRotationIsNotDrift(t *testing.T) {
	base := newProfileTestBase(t)
	ctx := context.Background()

	digest1, _, err := base.ApplyProfile(ctx, testDesired(7), "token-old")
	if err != nil {
		t.Fatalf("apply with old token: %v", err)
	}
	digest2, changed, err := base.ApplyProfile(ctx, testDesired(7), "token-new")
	if err != nil {
		t.Fatalf("apply with new token: %v", err)
	}
	if digest1 != digest2 || changed {
		t.Fatalf("换 token 影响了 digest/changed：%s→%s changed=%v", digest1, digest2, changed)
	}
	env, err := os.ReadFile(filepath.Join(base.profileDir(), ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(env), "GAK_HERMES_MODEL_TOKEN=token-new") {
		t.Fatalf("token 未轮换：%s", env)
	}
	if strings.Count(string(env), "GAK_HERMES_MODEL_TOKEN=") != 1 {
		t.Fatalf("受管键被重复追加：\n%s", env)
	}
}

// 员工在 .env 里的自有变量不能被 manager 抹掉（隐私与本机自主权承诺）。
func TestUpsertEnvPreservesEmployeeKeys(t *testing.T) {
	base := newProfileTestBase(t)
	envPath := filepath.Join(base.profileDir(), ".env")
	if err := os.WriteFile(envPath, []byte("MY_OWN_KEY=keep-me\nGAK_HERMES_MODEL_TOKEN=stale\nEDITOR=vim\n"), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	if _, _, err := base.ApplyProfile(context.Background(), testDesired(7), "token-fresh"); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)
	for _, want := range []string{"MY_OWN_KEY=keep-me", "EDITOR=vim", "GAK_HERMES_MODEL_TOKEN=token-fresh"} {
		if !strings.Contains(content, want) {
			t.Fatalf("缺少 %q，实际：\n%s", want, content)
		}
	}
	if strings.Contains(content, "stale") {
		t.Fatalf("旧 token 未被替换：\n%s", content)
	}
}

// ≤批3 的机器上 .env 里躺着 manager 写的 OPENAI_API_KEY。它取不到企业代理的
// 凭据（上游按 host 门控），留着还有反向风险：策略若把 base_url 指向 openai.com，
// 这个已吊销的企业 token 就会被发出去。升级后第一次渲染就该清掉，
// 且不能等到某次 token 轮换才顺带清。
func TestApplyProfileRemovesLegacyOpenAIKey(t *testing.T) {
	base := newProfileTestBase(t)
	envPath := filepath.Join(base.profileDir(), ".env")
	if err := os.WriteFile(envPath, []byte("OPENAI_API_KEY=old-enterprise-token\nMY_OWN_KEY=keep-me\n"), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	// modelToken 为空 = 本轮不轮换凭据，遗留键仍必须被清掉
	if _, _, err := base.ApplyProfile(context.Background(), testDesired(7), ""); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "OPENAI_API_KEY") {
		t.Fatalf("批3 遗留的受管键未被清理：\n%s", content)
	}
	if !strings.Contains(content, "MY_OWN_KEY=keep-me") {
		t.Fatalf("清理波及了员工自有变量：\n%s", content)
	}
}

// 非企业代理模式不该渲染 providers 段，也不该指定 provider——那会把员工
// 自己配的模型来源顶掉。
func TestApplyProfileOmitsProviderWhenNotCompanyProxy(t *testing.T) {
	base := newProfileTestBase(t)
	desired := testDesired(7)
	desired.Model.Mode = "byo"

	if _, _, err := base.ApplyProfile(context.Background(), desired, ""); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	rendered := string(config)
	for _, unwanted := range []string{"providers:", managedProviderName, "key_env"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("非企业代理模式仍渲染了 %q：\n%s", unwanted, rendered)
		}
	}
}

// M2 技能市场：ApplySkills 把 tarball 解压进 orchestrator profile 与 worker
// profile 的 skills/<name>/ 目录；../ 逃逸被 safeJoin 拒。
func TestApplySkillsExtractsToProfileTrees(t *testing.T) {
	base := newProfileTestBase(t)
	ctx := context.Background()

	// 造一个含 SKILL.md 的 tar.gz（artifact 包规范：顶层目录名，stripTop 会剥掉）
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("---\nname: weekly-report\n---\nProcedure\n")
	if err := tw.WriteHeader(&tar.Header{Name: "weekly-report/SKILL.md", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()

	skill := protocol.DesiredStateSkill{ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: "x", SourceURL: "/api/v1/ai/hermes/device/skills/5/download"}
	if err := base.ApplySkills(ctx, []SkillBundle{{Skill: skill, Tarball: buf.Bytes()}}); err != nil {
		t.Fatalf("apply skills: %v", err)
	}

	// orchestrator profile
	got, err := os.ReadFile(filepath.Join(base.profileDir(), "skills", "weekly-report", "SKILL.md"))
	if err != nil || !strings.Contains(string(got), "weekly-report") {
		t.Fatalf("orchestrator skill not extracted: %v\n%s", err, got)
	}

	// worker profile（先建一个 worker 目录）
	workerDir := filepath.Join(base.profileDir(), "profiles", "worker-a")
	if err := os.MkdirAll(workerDir, 0o700); err != nil {
		t.Fatalf("mkdir worker: %v", err)
	}
	if err := base.ApplySkills(ctx, []SkillBundle{{Skill: skill, Tarball: buf.Bytes()}}); err != nil {
		t.Fatalf("apply skills to worker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workerDir, "skills", "weekly-report", "SKILL.md")); err != nil {
		t.Fatalf("worker skill not extracted: %v", err)
	}
}

// 不安全技能名（路径逃逸）必须拒。
func TestApplySkillsRejectsUnsafeName(t *testing.T) {
	base := newProfileTestBase(t)
	skill := protocol.DesiredStateSkill{ID: 5, Name: "../escape", Version: "1.0.0", SHA256: "x", SourceURL: "/api/v1/ai/hermes/device/skills/5/download"}
	if err := base.ApplySkills(context.Background(), []SkillBundle{{Skill: skill, Tarball: []byte("garbage")}}); err == nil {
		t.Fatal("unsafe skill name must be rejected")
	}
}

// M3 MCP 管理：有 McpServers 时渲染 mcp_servers 块 + 剔除 no_mcp；无则字节不变。
func TestRenderManagedConfigIncludesMcpServers(t *testing.T) {
	// 无 MCP：P1 行为不变，保留 no_mcp
	base := newProfileTestBase(t)
	plain := testDesired(7)
	if _, _, err := base.ApplyProfile(context.Background(), plain, ""); err != nil {
		t.Fatalf("apply plain profile: %v", err)
	}
	plainConfig, _ := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if !strings.Contains(string(plainConfig), "no_mcp") {
		t.Fatal("no-mcp baseline should keep no_mcp")
	}
	if strings.Contains(string(plainConfig), "mcp_servers") {
		t.Fatal("no-mcp profile should not render mcp_servers")
	}

	// 有 MCP：渲染 mcp_servers 块 + 剔除 no_mcp
	mcp := testDesired(8)
	mcp.McpServers = []protocol.DesiredStateMcpServer{{
		ID: 3, Name: "company-crm", Type: "sse",
		URL: "https://crm.internal/v1/sse",
		Headers: map[string]string{"Authorization": "Bearer ${MCP_COMPANY_CRM_API_KEY}"},
	}}
	if _, _, err := base.ApplyProfile(context.Background(), mcp, ""); err != nil {
		t.Fatalf("apply mcp profile: %v", err)
	}
	config, _ := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if !strings.Contains(string(config), "mcp_servers:") {
		t.Fatal("mcp profile should render mcp_servers block")
	}
	if !strings.Contains(string(config), "company-crm") {
		t.Fatal("mcp block should contain server name")
	}
	if strings.Contains(string(config), "no_mcp") {
		t.Fatal("mcp profile should drop no_mcp (MCP ON)")
	}
	if !strings.Contains(string(config), `"Bearer ${MCP_COMPANY_CRM_API_KEY}"`) {
		t.Fatal("mcp block should carry ${VAR} header template")
	}
}
