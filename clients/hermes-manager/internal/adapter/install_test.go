package adapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// fakeProcess 记录进程操作。
type fakeProcess struct {
	runningState bool
	calls        []string
}

func (f *fakeProcess) start(context.Context) error {
	f.calls = append(f.calls, "start")
	f.runningState = true
	return nil
}

func (f *fakeProcess) stop(context.Context) error {
	f.calls = append(f.calls, "stop")
	f.runningState = false
	return nil
}

func (f *fakeProcess) running(context.Context) (bool, error) { return f.runningState, nil }

// fakeRunner 断言命令串并模拟 uv/hermes 行为：uv venv 时建出假 venv 目录。
type fakeRunner struct {
	home     string
	commands [][]string
	failOn   string // 命令名含此串则失败
}

// joined 把记录的命令拉平成多行文本，便于断言「执行了什么、顺序如何」。
func (f *fakeRunner) joined() string {
	lines := make([]string, 0, len(f.commands))
	for _, command := range f.commands {
		lines = append(lines, strings.Join(command, " "))
	}
	return strings.Join(lines, "\n")
}

func (f *fakeRunner) Run(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	full := append([]string{name}, args...)
	f.commands = append(f.commands, full)
	joined := strings.Join(full, " ")
	if f.failOn != "" && strings.Contains(joined, f.failOn) {
		return nil, errors.New("simulated failure: " + f.failOn)
	}
	// uv venv：在工作目录里造出 venv 与假 hermes，可执行文件存在性由后续步骤依赖
	if strings.Contains(joined, " venv ") || (len(args) > 0 && args[0] == "venv") {
		binDir := filepath.Join(dir, ".venv", "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(binDir, "hermes"), []byte("#!/bin/sh\necho stub"), 0o755); err != nil {
			return nil, err
		}
	}
	return []byte("ok"), nil
}

// stubArchive 造一个带顶层目录的 tar.gz（模拟 GitHub 源码包形态）。
func stubArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newTestBase(t *testing.T, runner Runner, proc process) *Base {
	t.Helper()
	base, err := newBase(Options{
		Home:              t.TempDir(),
		HTTPClient:        &http.Client{Timeout: 10 * time.Second},
		Runner:            runner,
		Now:               func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) },
		AllowInsecureHTTP: true, // httptest 是 http，测试里显式打开
	}, proc)
	if err != nil {
		t.Fatalf("new base: %v", err)
	}
	return base
}

func seedHermesBinary(t *testing.T, base *Base, version string) {
	t.Helper()
	binDir := filepath.Dir(base.hermesBinary(version))
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binary dir: %v", err)
	}
	if err := os.WriteFile(base.hermesBinary(version), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
}

func TestInstallPipelineWithStubArtifact(t *testing.T) {
	archive := stubArchive(t, map[string]string{
		"hermes-agent-abc/pyproject.toml": "[project]\nname = \"hermes\"\n",
		"hermes-agent-abc/cli.py":         "print('hi')\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	runner := &fakeRunner{}
	proc := &fakeProcess{runningState: true}
	base := newTestBase(t, runner, proc)
	runner.home = base.home
	// 预置 uv，绕开引导下载（引导有独立校验逻辑，别混进全链测试）
	if err := os.WriteFile(filepath.Join(base.home, "tools", uvBinaryName()), []byte("stub"), 0o755); err != nil {
		t.Fatalf("seed uv: %v", err)
	}

	artifact := protocol.DesiredStateArtifact{
		ID: 7, Version: "v2026.7.20", SHA256: digestOf(archive), SourceURL: server.URL + "/hermes.tar.gz",
	}
	if err := base.Install(context.Background(), artifact); err != nil {
		t.Fatalf("install: %v", err)
	}

	// 顶层目录被剥掉：源码直接落在版本目录
	if _, err := os.Stat(filepath.Join(base.versionDir("v2026.7.20"), "pyproject.toml")); err != nil {
		t.Fatalf("extracted source missing: %v", err)
	}
	// 指针切换
	version, err := base.InstalledVersion()
	if err != nil || version != "v2026.7.20" {
		t.Fatalf("installed version = %q err=%v", version, err)
	}
	// 旧进程被停（切指针前）
	stopSeen := false
	for _, call := range proc.calls {
		if call == "stop" {
			stopSeen = true
		}
	}
	if !stopSeen {
		t.Fatalf("process must be stopped before switch, calls = %v", proc.calls)
	}
	// 命令序列：venv → pip install → 健康检查
	var joined []string
	for _, cmd := range runner.commands {
		joined = append(joined, strings.Join(cmd, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{"venv --python 3.11", "pip install", "--version"} {
		if !strings.Contains(all, want) {
			t.Fatalf("command sequence missing %q:\n%s", want, all)
		}
	}
}

func TestInstallRejectsDigestMismatch(t *testing.T) {
	archive := stubArchive(t, map[string]string{"x/pyproject.toml": "x"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	base := newTestBase(t, &fakeRunner{}, &fakeProcess{})
	err := base.Install(context.Background(), protocol.DesiredStateArtifact{
		Version: "v1", SHA256: strings.Repeat("a", 64), SourceURL: server.URL,
	})
	if CodeOf(err) != ErrCodeDigestMismatch {
		t.Fatalf("error code = %s, want DIGEST_MISMATCH (err=%v)", CodeOf(err), err)
	}
	// 失败不得切指针
	if version, _ := base.InstalledVersion(); version != "" {
		t.Fatalf("pointer must not switch on failure, got %q", version)
	}
}

func TestInstallRejectsHTTPWithoutOptIn(t *testing.T) {
	base, err := newBase(Options{Home: t.TempDir(), Runner: &fakeRunner{}}, &fakeProcess{})
	if err != nil {
		t.Fatalf("new base: %v", err)
	}
	installErr := base.Install(context.Background(), protocol.DesiredStateArtifact{
		Version: "v1", SHA256: strings.Repeat("a", 64), SourceURL: "http://artifacts.internal/hermes.tar.gz",
	})
	if CodeOf(installErr) != ErrCodeDownloadFailed || !strings.Contains(installErr.Error(), "https") {
		t.Fatalf("plain http must be rejected without opt-in, got %v", installErr)
	}
}

func TestInstallRejectsArtifactVersionOutsideManagedDirectory(t *testing.T) {
	dangerousVersions := []string{
		"../profile",
		`..\profile`,
		"/tmp/hermes",
		"v1/child",
		".",
		"-leading-dash",
		"v1.",
		"v1-",
	}

	for _, version := range dangerousVersions {
		t.Run(version, func(t *testing.T) {
			runner := &fakeRunner{}
			proc := &fakeProcess{runningState: true}
			base := newTestBase(t, runner, proc)
			sentinel := filepath.Join(base.profileDir(), "must-survive")
			if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
				t.Fatalf("seed profile sentinel: %v", err)
			}

			err := base.Install(context.Background(), protocol.DesiredStateArtifact{
				ID: 9, Version: version, SHA256: strings.Repeat("a", 64), SourceURL: "https://example.invalid/hermes.tar.gz",
			})

			if CodeOf(err) != ErrCodeExtractFailed {
				t.Fatalf("unsafe version %q error = %v, want %s", version, err, ErrCodeExtractFailed)
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Fatalf("unsafe version %q touched managed profile: %v", version, err)
			}
			if len(runner.commands) != 0 || len(proc.calls) != 0 {
				t.Fatalf("unsafe version %q must fail before download/process control: commands=%v process=%v", version, runner.commands, proc.calls)
			}
		})
	}
}

func TestDownloadRejectsHTTPSDowngradeRedirect(t *testing.T) {
	downgradedRequest := make(chan struct{}, 1)
	insecureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case downgradedRequest <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte("untrusted artifact"))
	}))
	defer insecureServer.Close()

	secureServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", insecureServer.URL+"/artifact.tar.gz")
		w.WriteHeader(http.StatusFound)
	}))
	defer secureServer.Close()

	base, err := newBase(Options{
		Home:              t.TempDir(),
		HTTPClient:        secureServer.Client(),
		AllowInsecureHTTP: true,
	}, &fakeProcess{})
	if err != nil {
		t.Fatalf("new base: %v", err)
	}
	_, downloadErr := base.download(context.Background(), secureServer.URL+"/artifact.tar.gz", strings.Repeat("a", 64), "artifact-*")
	if downloadErr == nil || !strings.Contains(downloadErr.Error(), "may not redirect") {
		t.Fatalf("HTTPS downgrade redirect error = %v, want rejection", downloadErr)
	}
	select {
	case <-downgradedRequest:
		t.Fatal("downgraded HTTP target must not be requested")
	default:
	}
}

func TestDownloadRejectsUnsupportedURLScheme(t *testing.T) {
	base := newTestBase(t, &fakeRunner{}, &fakeProcess{})
	_, err := base.download(context.Background(), "file:///tmp/hermes.tar.gz", strings.Repeat("a", 64), "artifact-*")
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("unsupported scheme error = %v, want rejection", err)
	}
}

func TestUpdateRecordsAndRestoresManagedProfileBackup(t *testing.T) {
	archive := stubArchive(t, map[string]string{
		"hermes-agent-next/pyproject.toml": "[project]\nname = \"hermes\"\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	runner := &fakeRunner{}
	proc := &fakeProcess{runningState: true}
	base := newTestBase(t, runner, proc)
	if err := os.WriteFile(filepath.Join(base.home, "tools", uvBinaryName()), []byte("stub"), 0o755); err != nil {
		t.Fatalf("seed uv: %v", err)
	}
	seedHermesBinary(t, base, "v-old")
	if err := base.savePointer(&currentPointer{Version: "v-old"}); err != nil {
		t.Fatalf("save old pointer: %v", err)
	}
	oldConfig := []byte("model: old\n")
	oldEnv := []byte("HERMES_INFERENCE_API_KEY=old-placeholder\n")
	if err := os.WriteFile(filepath.Join(base.profileDir(), "config.yaml"), oldConfig, 0o600); err != nil {
		t.Fatalf("seed old config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base.profileDir(), ".env"), oldEnv, 0o600); err != nil {
		t.Fatalf("seed old env: %v", err)
	}

	artifact := protocol.DesiredStateArtifact{
		ID: 8, Version: "v-new", SHA256: digestOf(archive), SourceURL: server.URL + "/hermes.tar.gz",
	}
	if err := base.Install(context.Background(), artifact); err != nil {
		t.Fatalf("update install: %v", err)
	}
	pointer, err := base.loadPointer()
	if err != nil {
		t.Fatalf("load pointer: %v", err)
	}
	if pointer.PreviousVersion != "v-old" || pointer.PreviousProfileBackup == "" || !safeProfileBackupName(pointer.PreviousProfileBackup) {
		t.Fatalf("update pointer = %+v", pointer)
	}
	backupDir := filepath.Join(base.home, "backups", pointer.PreviousProfileBackup)
	if filepath.Dir(backupDir) != filepath.Join(base.home, "backups") {
		t.Fatalf("backup escaped managed root: %s", backupDir)
	}
	for name, want := range map[string][]byte{"config.yaml": oldConfig, ".env": oldEnv} {
		got, readErr := os.ReadFile(filepath.Join(backupDir, name))
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("backup %s = %q err=%v, want %q", name, got, readErr, want)
		}
	}

	if err := os.WriteFile(filepath.Join(base.profileDir(), "config.yaml"), []byte("model: new\n"), 0o600); err != nil {
		t.Fatalf("write new config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base.profileDir(), ".env"), []byte("HERMES_INFERENCE_API_KEY=new-placeholder\n"), 0o600); err != nil {
		t.Fatalf("write new env: %v", err)
	}
	if err := base.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for name, want := range map[string][]byte{"config.yaml": oldConfig, ".env": oldEnv} {
		got, readErr := os.ReadFile(filepath.Join(base.profileDir(), name))
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("restored %s = %q err=%v, want %q", name, got, readErr, want)
		}
	}
	if version, versionErr := base.InstalledVersion(); versionErr != nil || version != "v-old" {
		t.Fatalf("rolled-back version = %q err=%v", version, versionErr)
	}
}

func TestRollbackRestoresAbsenceFromEmptyProfileBackup(t *testing.T) {
	base := newTestBase(t, &fakeRunner{}, &fakeProcess{runningState: true})
	seedHermesBinary(t, base, "v-old")
	seedHermesBinary(t, base, "v-new")
	backupName := "20260731-120000-empty"
	if err := os.Mkdir(filepath.Join(base.home, "backups", backupName), 0o700); err != nil {
		t.Fatalf("mkdir empty backup: %v", err)
	}
	if err := base.savePointer(&currentPointer{
		Version: "v-new", PreviousVersion: "v-old", PreviousProfileBackup: backupName,
	}); err != nil {
		t.Fatalf("save pointer: %v", err)
	}
	for _, name := range []string{"config.yaml", ".env"} {
		if err := os.WriteFile(filepath.Join(base.profileDir(), name), []byte("new"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	if err := base.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for _, name := range []string{"config.yaml", ".env"} {
		if _, err := os.Stat(filepath.Join(base.profileDir(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s must be removed when absent from snapshot, err=%v", name, err)
		}
	}
}

func TestRollbackRejectsProfileBackupPathEscape(t *testing.T) {
	proc := &fakeProcess{runningState: true}
	base := newTestBase(t, &fakeRunner{}, proc)
	seedHermesBinary(t, base, "v-old")
	seedHermesBinary(t, base, "v-new")

	if err := base.savePointer(&currentPointer{
		Version: "v-new", PreviousVersion: "v-old", PreviousProfileBackup: "../outside",
	}); err == nil {
		t.Fatal("savePointer must reject a backup path outside backups")
	}
	// current.json 是本机文件，即使被篡改也不能让 Rollback 读任意路径。
	tampered := []byte("{\"version\":\"v-new\",\"previous_version\":\"v-old\",\"previous_profile_backup\":\"../../outside\"}\n")
	if err := os.WriteFile(filepath.Join(base.home, "current.json"), tampered, 0o600); err != nil {
		t.Fatalf("write tampered pointer: %v", err)
	}

	if err := base.Rollback(context.Background()); CodeOf(err) != ErrCodeRollbackFailed {
		t.Fatalf("rollback error = %v, want ROLLBACK_FAILED", err)
	}
	if len(proc.calls) != 0 {
		t.Fatalf("unsafe backup must be rejected before stopping process, calls=%v", proc.calls)
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	evil := stubArchive(t, map[string]string{
		"top/../../evil.txt": "pwned",
	})
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "evil.tar.gz")
	if err := os.WriteFile(archivePath, evil, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := extractArchive(archivePath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("path traversal entry must be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("traversal file must not be written")
	}
}

func TestRollbackSwitchesPointerAndRestarts(t *testing.T) {
	proc := &fakeProcess{runningState: true}
	base := newTestBase(t, &fakeRunner{}, proc)

	// 造两个版本与指针：current v2（previous v1）
	for _, version := range []string{"v1", "v2"} {
		seedHermesBinary(t, base, version)
	}
	if err := base.savePointer(&currentPointer{Version: "v2", PreviousVersion: "v1"}); err != nil {
		t.Fatalf("save pointer: %v", err)
	}
	legacyConfig := []byte("legacy pointer keeps current profile\n")
	if err := os.WriteFile(filepath.Join(base.profileDir(), "config.yaml"), legacyConfig, 0o600); err != nil {
		t.Fatalf("seed legacy profile: %v", err)
	}

	if err := base.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	version, _ := base.InstalledVersion()
	if version != "v1" {
		t.Fatalf("after rollback version = %q, want v1", version)
	}
	if got, err := os.ReadFile(filepath.Join(base.profileDir(), "config.yaml")); err != nil || !bytes.Equal(got, legacyConfig) {
		t.Fatalf("legacy pointer must leave profile unchanged, got=%q err=%v", got, err)
	}
	// 序列：stop → start
	if len(proc.calls) < 2 || proc.calls[len(proc.calls)-2] != "stop" || proc.calls[len(proc.calls)-1] != "start" {
		t.Fatalf("rollback process calls = %v, want ... stop, start", proc.calls)
	}

	// 没有上一版本时拒绝
	if err := base.Rollback(context.Background()); CodeOf(err) != ErrCodeRollbackFailed {
		t.Fatalf("second rollback = %v, want ROLLBACK_FAILED", err)
	}
}
