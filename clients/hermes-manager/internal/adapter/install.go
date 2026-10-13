package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// Base 是跨平台共享的适配器主体：安装/更新/回滚/受管 profile 都在这里，
// 平台差异（进程监管）由嵌入方（darwin/windows 适配器）实现 process 接口。
type Base struct {
	home       string
	httpClient *http.Client
	runner     Runner
	process    process
	now        func() time.Time
	// allowInsecureHTTP 允许 http 制品源（仅内网自托管场景显式打开）。
	allowInsecureHTTP bool
}

// process 是平台进程监管面，由 process_darwin / process_windows 提供。
type process interface {
	start(ctx context.Context) error
	stop(ctx context.Context) error
	running(ctx context.Context) (bool, error)
}

type Options struct {
	Home              string
	HTTPClient        *http.Client
	Runner            Runner
	Now               func() time.Time
	AllowInsecureHTTP bool
}

func newBase(options Options, proc process) (*Base, error) {
	if options.Home == "" {
		return nil, errors.New("adapter home directory is required")
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	if options.Runner == nil {
		options.Runner = ExecRunner{}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	for _, sub := range []string{"versions", "profiles", "backups", "logs", "tools"} {
		if err := os.MkdirAll(filepath.Join(options.Home, sub), 0o700); err != nil {
			return nil, err
		}
	}
	// orchestrator profile 目录：home/profiles/company
	if err := os.MkdirAll(filepath.Join(options.Home, "profiles", orchestratorProfileName), 0o700); err != nil {
		return nil, err
	}
	base := &Base{
		home:              options.Home,
		httpClient:        options.HTTPClient,
		runner:            options.Runner,
		process:           proc,
		now:               options.Now,
		allowInsecureHTTP: options.AllowInsecureHTTP,
	}
	// Kanban 里程碑布局迁移：把 phase-1 的 home/profile 内容搬到
	// home/profiles/company。纯新设备无旧布局则跳过；失败不阻断构造（配置
	// 渲染时会再次尝试写入新位置，旧位置残留只是历史数据）。
	if err := base.migrateLegacyProfile(); err != nil {
		return nil, err
	}
	return base, nil
}

func (b *Base) versionDir(version string) string {
	return filepath.Join(b.home, "versions", version)
}

// hermesBinary 返回某版本 venv 里的 hermes 可执行文件路径。
func (b *Base) hermesBinary(version string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(b.versionDir(version), ".venv", "Scripts", "hermes.exe")
	}
	return filepath.Join(b.versionDir(version), ".venv", "bin", "hermes")
}

// orchestratorProfileName 是受管 orchestrator profile 名。与 -p company 对齐，
// 也是 upstream 把 HERMES_HOME 设为 <root>/profiles/<name> 时的 profile 名。
const orchestratorProfileName = "company"

// profileDir 是受管 orchestrator 的 HERMES_HOME：home/profiles/company。
//
// Kanban 里程碑起迁到 home 根布局：worker profile 于 home/profiles/<worker>、
// board 于 home/kanban/boards/<slug>。把 orchestrator 也作为 home/profiles/company
// 让 HERMES_HOME 落在 <root>/profiles/<name> 形态，upstream get_default_hermes_root
// 会取其 grandparent 作为 root，从而 board/worker 与 orchestrator 共享同一 root。
// 存量设备（home/profile 已有内容）首次访问时把内容迁移到新位置。
func (b *Base) profileDir() string {
	return filepath.Join(b.home, "profiles", orchestratorProfileName)
}

// migrateLegacyProfile 把 phase-1 的 home/profile 内容迁移到 home/profiles/company。
// 只在旧位置存在而新位置尚未建立时执行一次；幂等。
func (b *Base) migrateLegacyProfile() error {
	legacy := filepath.Join(b.home, "profile")
	target := b.profileDir()
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return nil // 无旧布局，纯新设备
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	// 目标已非空（如升级后回滚留下的）则不动，避免覆盖新数据
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(target, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("migrate legacy profile entry %s: %w", entry.Name(), err)
		}
	}
	// 清掉空目录
	_ = os.Remove(legacy)
	return nil
}

func (b *Base) InstalledVersion() (string, error) {
	pointer, err := b.loadPointer()
	if err != nil {
		return "", Coded(ErrCodeNotInstalled, err)
	}
	if pointer == nil {
		return "", nil
	}
	return pointer.Version, nil
}

// Install 按文档 §9.3 的顺序执行；任何一步失败都不改变 current 指针，
// 当前活跃版本照常可用。
func (b *Base) Install(ctx context.Context, artifact protocol.DesiredStateArtifact) error {
	if artifact.Version == "" || artifact.SHA256 == "" || artifact.SourceURL == "" {
		return Coded(ErrCodeDownloadFailed, errors.New("artifact is missing version, digest or source url"))
	}
	if !safeArtifactVersion(artifact.Version) {
		return Coded(ErrCodeExtractFailed, errors.New("artifact version is not a safe directory name"))
	}
	previous, err := b.loadPointer()
	if err != nil {
		return Coded(ErrCodeNotInstalled, err)
	}

	// 1. 磁盘空间
	if free, err := freeDiskBytes(b.home); err == nil && free < minFreeBytes {
		return Coded(ErrCodeDiskSpaceLow, fmt.Errorf("free disk %d bytes < required %d", free, int64(minFreeBytes)))
	}

	// 2. 更新前备份受管 profile 元数据（只拷 config.yaml/.env；
	//    对话、memory 不在 profile 受管文件之列，天然不进备份）。
	profileBackup := ""
	if previous != nil && previous.Version != artifact.Version {
		profileBackup, err = b.backupProfile()
		if err != nil {
			return Coded(ErrCodeProfileFailed, err)
		}
	}

	// 3. 下载 + 4. 校验（download 内部强制 sha256）
	archivePath, err := b.download(ctx, artifact.SourceURL, artifact.SHA256, "hermes-artifact-*")
	if err != nil {
		if isDigestMismatch(err) {
			return Coded(ErrCodeDigestMismatch, err)
		}
		return Coded(ErrCodeDownloadFailed, err)
	}
	defer os.Remove(archivePath)

	// 5. 解压到版本目录（先清残留，失败目录不复用）
	targetDir := b.versionDir(artifact.Version)
	if err := os.RemoveAll(targetDir); err != nil {
		return Coded(ErrCodeExtractFailed, err)
	}
	if err := extractArchive(archivePath, targetDir); err != nil {
		_ = os.RemoveAll(targetDir)
		return Coded(ErrCodeExtractFailed, err)
	}

	// 6. uv venv + 依赖安装
	uvPath, err := b.ensureUV(ctx)
	if err != nil {
		return err
	}
	if _, err := b.runner.Run(ctx, targetDir, nil, uvPath, "venv", "--python", "3.11", ".venv"); err != nil {
		return Coded(ErrCodeVenvFailed, err)
	}
	if _, err := b.runner.Run(ctx, targetDir, nil, uvPath, "pip", "install", "--python", filepath.Join(".venv", pythonRelPath()), "-e", "."); err != nil {
		return Coded(ErrCodeVenvFailed, err)
	}

	// 7. 健康检查：装出来的 hermes 至少能自报版本
	if _, err := b.runner.Run(ctx, targetDir, nil, b.hermesBinary(artifact.Version), "--version"); err != nil {
		return Coded(ErrCodeHealthCheckFailed, err)
	}

	// 8. 停旧 → 切指针。启动交给 reconcile 的进程收敛步（保持单一职责）。
	if err := b.process.stop(ctx); err != nil {
		return Coded(ErrCodeStopFailed, err)
	}
	pointer := &currentPointer{
		Version:    artifact.Version,
		ArtifactID: artifact.ID,
		SHA256:     artifact.SHA256,
	}
	if previous != nil && previous.Version != artifact.Version {
		pointer.PreviousVersion = previous.Version
		pointer.PreviousProfileBackup = profileBackup
	}
	if err := b.savePointer(pointer); err != nil {
		return Coded(ErrCodeNotInstalled, err)
	}
	return nil
}

// Rollback 把指针切回上一版本。失败版本目录保留在 versions/ 供本机诊断
// （文档 §9.3 第 6 步）。
func (b *Base) Rollback(ctx context.Context) error {
	pointer, err := b.loadPointer()
	if err != nil || pointer == nil {
		return Coded(ErrCodeRollbackFailed, errors.New("nothing installed"))
	}
	if pointer.PreviousVersion == "" {
		return Coded(ErrCodeRollbackFailed, errors.New("no previous version to roll back to"))
	}
	if _, err := os.Stat(b.hermesBinary(pointer.PreviousVersion)); err != nil {
		return Coded(ErrCodeRollbackFailed, fmt.Errorf("previous version binary missing: %w", err))
	}
	profileSnapshot, err := b.loadProfileBackup(pointer.PreviousProfileBackup)
	if err != nil {
		return Coded(ErrCodeRollbackFailed, err)
	}
	if err := b.process.stop(ctx); err != nil {
		return Coded(ErrCodeStopFailed, err)
	}
	if err := b.restoreProfileBackup(profileSnapshot); err != nil {
		return Coded(ErrCodeRollbackFailed, err)
	}
	if err := b.savePointer(&currentPointer{
		Version:    pointer.PreviousVersion,
		ArtifactID: 0, // 回滚后的版本未必仍是某制品的最新引用，置零让下轮 reconcile 决策
		SHA256:     "",
	}); err != nil {
		return Coded(ErrCodeRollbackFailed, err)
	}
	return b.Start(ctx)
}

func (b *Base) Start(ctx context.Context) error {
	version, err := b.InstalledVersion()
	if err != nil {
		return err
	}
	if version == "" {
		return Coded(ErrCodeNotInstalled, errors.New("hermes is not installed"))
	}
	if err := b.process.start(ctx); err != nil {
		return Coded(ErrCodeStartFailed, err)
	}
	return nil
}

func (b *Base) Stop(ctx context.Context) error {
	if err := b.process.stop(ctx); err != nil {
		return Coded(ErrCodeStopFailed, err)
	}
	return nil
}

func (b *Base) Restart(ctx context.Context) error {
	if err := b.Stop(ctx); err != nil {
		return err
	}
	return b.Start(ctx)
}

func (b *Base) Running(ctx context.Context) (bool, error) {
	return b.process.running(ctx)
}

// backupProfile 把受管配置元数据拷进 backups/<ts>-<random>/，
// 返回的只是 backups/ 下的直接子目录名。即使两个文件都不存在也保留
// 空快照目录，回滚时据此删掉更新后新生成的受管文件。
func (b *Base) backupProfile() (backupName string, err error) {
	entries := []string{"config.yaml", ".env"}
	stamp := b.now().UTC().Format("20060102-150405")
	backupRoot := filepath.Join(b.home, "backups")
	backupDir, err := os.MkdirTemp(backupRoot, stamp+"-")
	if err != nil {
		return "", err
	}
	backupName = filepath.Base(backupDir)
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(backupDir)
		}
	}()
	for _, name := range entries {
		data, err := os.ReadFile(filepath.Join(b.profileDir(), name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := atomicWriteFile(backupDir, name, data, 0o600); err != nil {
			return "", err
		}
	}
	committed = true
	return backupName, nil
}

type managedProfileSnapshot struct {
	name   string
	data   []byte
	exists bool
}

// loadProfileBackup 在停进程前完成路径与文件类型校验。旧指针没有
// previous_profile_backup 时返回 nil，保持原有「只切版本」的回滚语义。
func (b *Base) loadProfileBackup(backupName string) ([]managedProfileSnapshot, error) {
	if backupName == "" {
		return nil, nil
	}
	if !safeProfileBackupName(backupName) {
		return nil, errors.New("profile backup is outside the backups directory")
	}
	backupRoot := filepath.Join(b.home, "backups")
	rootInfo, err := os.Lstat(backupRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("profile backups root is not a regular directory")
	}
	backupDir := filepath.Join(backupRoot, backupName)
	dirInfo, err := os.Lstat(backupDir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("profile backup is not a regular directory")
	}

	snapshot := make([]managedProfileSnapshot, 0, 2)
	for _, name := range []string{"config.yaml", ".env"} {
		path := filepath.Join(backupDir, name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			snapshot = append(snapshot, managedProfileSnapshot{name: name})
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("profile backup file %s is not regular", name)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		snapshot = append(snapshot, managedProfileSnapshot{name: name, data: data, exists: true})
	}
	return snapshot, nil
}

func (b *Base) restoreProfileBackup(snapshot []managedProfileSnapshot) error {
	for _, file := range snapshot {
		if file.exists {
			if err := atomicWriteFile(b.profileDir(), file.name, file.data, 0o600); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(filepath.Join(b.profileDir(), file.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func pythonRelPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("Scripts", "python.exe")
	}
	return filepath.Join("bin", "python")
}

func isDigestMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "sha256 mismatch")
}
