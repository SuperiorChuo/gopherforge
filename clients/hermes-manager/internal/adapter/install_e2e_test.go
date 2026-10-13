//go:build darwin || linux

package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// 干验证 canary：用**真实**上游制品把安装链跑通，回答单测答不了的问题——
// 上游 tar 包的目录形状、uv 引导、venv 与 -e 安装、装出来的 hermes 认不认
// 我们渲染的 config.yaml。其余单测全用 stub 制品和 fakeRunner，验的是控制流；
// 这里验的是「对真实上游是否成立」。
//
// 默认跳过：要联网、要下 69MB 制品、要 uv 拉 Python 3.11，耗时以分钟计，
// 不该拖慢日常 go test。打开方式：
//
//	GAK_HERMES_E2E=1 go test -v -timeout 40m -run E2E ./internal/adapter
//
// 设 GAK_HERMES_E2E_HOME 可把安装目录留在固定位置供事后翻查（默认用临时目录，
// 测完即删）。
const (
	e2eDefaultVersion   = "v2026.7.20"
	e2eDefaultCommit    = "3ef6bbd20126"
	e2eDefaultSHA256    = "285f3fc134ff466a90065e1517801a68993733b807158ee8f32aa01613786990"
	e2eDefaultSourceURL = "https://codeload.github.com/NousResearch/hermes-agent/tar.gz/refs/tags/v2026.7.20"
)

func e2eEnvOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func requireE2E(t *testing.T) string {
	t.Helper()
	if os.Getenv("GAK_HERMES_E2E") != "1" {
		t.Skip("干验证默认跳过；GAK_HERMES_E2E=1 打开（需联网 + 数分钟）")
	}
	home := os.Getenv("GAK_HERMES_E2E_HOME")
	if home == "" {
		return t.TempDir()
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("准备 home: %v", err)
	}
	return home
}

func e2eArtifact() protocol.DesiredStateArtifact {
	return protocol.DesiredStateArtifact{
		ID:        1,
		Version:   e2eEnvOr("GAK_HERMES_E2E_VERSION", e2eDefaultVersion),
		Commit:    e2eEnvOr("GAK_HERMES_E2E_COMMIT", e2eDefaultCommit),
		SHA256:    e2eEnvOr("GAK_HERMES_E2E_SHA256", e2eDefaultSHA256),
		SourceURL: e2eEnvOr("GAK_HERMES_E2E_URL", e2eDefaultSourceURL),
	}
}

// e2eDesiredState 造一份代表性策略：企业代理 + P1 工具白名单，
// 与批3 对上游校准的键名一一对应。
func e2eDesiredState(artifact protocol.DesiredStateArtifact) *protocol.DesiredState {
	return &protocol.DesiredState{
		UnsignedDesiredState: protocol.UnsignedDesiredState{
			SchemaVersion: 1,
			Revision:      7,
			DesiredState:  "active",
			Profile:       "company",
			Hermes:        artifact,
			Model: protocol.DesiredStateModel{
				Mode:    "company_proxy",
				BaseURL: "https://ai.example.internal/v1",
				Model:   "hermes-4-70b",
			},
			Toolsets: []string{"clarify", "memory", "todo", "no_mcp"},
		},
	}
}

func TestE2EInstallRealArtifact(t *testing.T) {
	home := requireE2E(t)
	artifact := e2eArtifact()
	t.Logf("home=%s\n制品 %s commit=%s\n源 %s", home, artifact.Version, artifact.Commit, artifact.SourceURL)

	adp, err := NewAdapter(Options{Home: home})
	if err != nil {
		t.Fatalf("构造适配器: %v", err)
	}

	// 安装链本身可能跑很久（uv 要拉 Python 3.11 + 装全部依赖）
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	started := time.Now()
	if err := adp.Install(ctx, artifact); err != nil {
		t.Fatalf("安装失败（错误码 %s）：%v", CodeOf(err), err)
	}
	t.Logf("安装耗时 %s", time.Since(started).Round(time.Second))

	// 指针切换是安装链的终点，切了才算这一版真的生效
	version, err := adp.InstalledVersion()
	if err != nil {
		t.Fatalf("读已装版本: %v", err)
	}
	if version != artifact.Version {
		t.Fatalf("已装版本 = %q，期望 %q", version, artifact.Version)
	}

	base := adp.(*Base)
	binary := base.hermesBinary(version)
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("venv 里没有 hermes 可执行文件 %s: %v", binary, err)
	}
	t.Logf("hermes 二进制 %s", binary)

	// §9.3 第 5 步写的是 `hermes version` + `hermes doctor`，install.go 实际只跑
	// `--version`。三种形式都探一遍，用真实二进制确定哪些成立。
	runner := ExecRunner{Timeout: 3 * time.Minute}
	for _, probe := range [][]string{{"--version"}, {"version"}, {"doctor"}} {
		output, probeErr := runner.Run(ctx, base.versionDir(version), nil, binary, probe...)
		status := "OK"
		if probeErr != nil {
			status = "FAIL"
		}
		t.Logf("健康检查探针 `hermes %s` → %s\n%s",
			strings.Join(probe, " "), status, e2eTail(string(output), 600))
	}
}

// 受管 profile 渲染出来的 config.yaml，真实 hermes 认不认——批3 的键名是照上游
// 源码校准的，从没让真的 hermes 读过一次。
func TestE2EManagedProfileAcceptedByHermes(t *testing.T) {
	home := requireE2E(t)
	artifact := e2eArtifact()

	adp, err := NewAdapter(Options{Home: home})
	if err != nil {
		t.Fatalf("构造适配器: %v", err)
	}
	version, err := adp.InstalledVersion()
	if err != nil || version == "" {
		t.Skipf("尚未安装（先跑 TestE2EInstallRealArtifact，并用 GAK_HERMES_E2E_HOME 固定目录）：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	digest, changed, err := adp.ApplyProfile(ctx, e2eDesiredState(artifact), "e2e-fake-token-not-a-real-credential")
	if err != nil {
		t.Fatalf("渲染 profile 失败（错误码 %s）：%v", CodeOf(err), err)
	}
	t.Logf("profile digest=%s changed=%v", digest, changed)

	base := adp.(*Base)
	rendered, err := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("读渲染结果: %v", err)
	}
	t.Logf("渲染的 config.yaml：\n%s", rendered)

	// 幂等：同一策略再渲染一次不得报 changed，否则漂移检测会天天误报
	if _, changedAgain, err := adp.ApplyProfile(ctx, e2eDesiredState(artifact), ""); err != nil {
		t.Fatalf("二次渲染失败: %v", err)
	} else if changedAgain {
		t.Fatal("同一策略二次渲染报 changed，漂移检测会持续误报")
	}

	// 让真实 hermes 带着受管 HERMES_HOME 读一次配置：配置解析不了的话，
	// 这里会以非零退出或报错信息暴露出来。
	runner := ExecRunner{Timeout: 5 * time.Minute}
	env := []string{"HERMES_HOME=" + base.profileDir()}
	output, err := runner.Run(ctx, base.versionDir(version), env, base.hermesBinary(version), "doctor")
	t.Logf("HERMES_HOME=%s hermes doctor → err=%v\n%s", base.profileDir(), err, e2eTail(string(output), 4000))
	if err != nil {
		t.Logf("注意：doctor 非零退出不必然等于配置不合法（可能是缺模型凭据等），看上面输出判断")
	}
}

// 企业凭据到底有没有被 hermes 取到——这是 doctor 答不了的问题。
//
// doctor 只查 base_url 在不在（实测报 `✓ API key or custom endpoint configured`），
// 而上游对取不到的 key 会兜底成字面量 "no-key-required" 照发不误。批3 用
// OPENAI_API_KEY 承载凭据的写法就是这样静默失效的：401 由企业代理返回，
// 本地三道关全绿。所以这里直接问上游的解析函数要结果。
func TestE2EModelTokenActuallyResolves(t *testing.T) {
	home := requireE2E(t)
	artifact := e2eArtifact()

	adp, err := NewAdapter(Options{Home: home})
	if err != nil {
		t.Fatalf("构造适配器: %v", err)
	}
	version, err := adp.InstalledVersion()
	if err != nil || version == "" {
		t.Skipf("尚未安装（先跑 TestE2EInstallRealArtifact，并用 GAK_HERMES_E2E_HOME 固定目录）：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const sentinel = "sentinel-token-0a1b2c3d"
	if _, _, err := adp.ApplyProfile(ctx, e2eDesiredState(artifact), sentinel); err != nil {
		t.Fatalf("渲染 profile: %v", err)
	}

	base := adp.(*Base)
	// 只打印解析出的 provider / base_url / 凭据来源与是否命中哨兵，不回显凭据本身。
	// 先走上游自己的 .env 加载再解析——CLI 启动时就是这个顺序，少了它
	// os.environ 里没有受管键，测出来的会是探针的假阴性而不是产品行为。
	const probe = `
import json
from hermes_cli.env_loader import load_hermes_dotenv
load_hermes_dotenv()
from hermes_cli.runtime_provider import resolve_runtime_provider
r = resolve_runtime_provider()
print(json.dumps({
    "provider": r.get("provider"),
    "base_url": r.get("base_url"),
    "source": r.get("source"),
    "api_key_is_sentinel": r.get("api_key") == "sentinel-token-0a1b2c3d",
    "api_key_is_placeholder": r.get("api_key") == "no-key-required",
}))
`
	python := filepath.Join(base.versionDir(version), ".venv", "bin", "python")
	runner := ExecRunner{Timeout: 5 * time.Minute}
	output, err := runner.Run(ctx, base.versionDir(version),
		[]string{"HERMES_HOME=" + base.profileDir()}, python, "-c", probe)
	if err != nil {
		t.Fatalf("调用上游解析函数失败: %v\n%s", err, e2eTail(string(output), 1500))
	}
	result := strings.TrimSpace(string(output))
	t.Logf("上游解析结果：%s", result)

	if strings.Contains(result, `"api_key_is_placeholder": true`) {
		t.Fatal("hermes 回落到 no-key-required：企业凭据没被取到，请求会被代理 401 拒绝且本地不报错")
	}
	if !strings.Contains(result, `"api_key_is_sentinel": true`) {
		t.Fatal("hermes 解析出的凭据不是 manager 写入的那一个——受管 .env 的键名与上游取值路径对不上")
	}
}

func e2eTail(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return "…（截断）…\n" + text[len(text)-limit:]
}
