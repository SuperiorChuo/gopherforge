package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// M2 技能市场：manager 把控制面下发的技能 tarball 直解压进受管 profile 的
// skills 树。上游 Hermes 从 <HERMES_HOME>/skills/<name>/SKILL.md 自动发现，
// 无需 config 引用（filesystem scan，skills_list 工具 ~30s TTL）。

// SkillBundle 是下载并校验过的技能 tarball，与签名 DesiredStateSkill 对应。
type SkillBundle struct {
	Skill   protocol.DesiredStateSkill
	Tarball []byte
}

// skillNamePattern 是技能名白名单（与 protocol.validateSkills 同构）。技能名
// 会成为 skills 目录名，必须文件系统安全。
var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

// ApplySkills 把每个 skill tarball 解压进 orchestrator profile 与每个 worker
// profile 的 skills/<name>/ 目录。extractArchive 内部走 safeJoin（防路径逃逸
// 与符号链接），绝不 shell 插值。幂等：目标已存在同内容时覆盖重写。
func (b *Base) ApplySkills(_ context.Context, bundles []SkillBundle) error {
	// 目标 profile 目录集合：orchestrator + 每个 worker
	targetDirs := []string{b.profileDir()}
	// worker 目录由 ApplyWorkerProfiles 在 profileDir()/profiles/<worker> 下
	// 建立；这里列出已存在的 worker profile（技能集与 orchestrator 相同）
	workersRoot := filepath.Join(b.profileDir(), "profiles")
	if entries, err := os.ReadDir(workersRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				targetDirs = append(targetDirs, filepath.Join(workersRoot, entry.Name()))
			}
		}
	}

	for _, bundle := range bundles {
		if !skillNamePattern.MatchString(bundle.Skill.Name) {
			return Coded(ErrCodeSkillInstallFailed, fmt.Errorf("unsafe skill name %q", bundle.Skill.Name))
		}
		for _, target := range targetDirs {
			if err := installSkillBundle(b, target, bundle); err != nil {
				return err
			}
		}
	}
	return nil
}

// installSkillBundle 把单个 tarball 解压进一个 profile 的 skills/<name>/。
func installSkillBundle(b *Base, profileDir string, bundle SkillBundle) error {
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	// 写临时文件（复用 download 的 temp 语义），解压到目标 skills/<name>
	tmp, err := os.CreateTemp(b.home, "skill-*")
	if err != nil {
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(bundle.Tarball); err != nil {
		_ = tmp.Close()
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	if err := tmp.Close(); err != nil {
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	dest := filepath.Join(profileDir, "skills", bundle.Skill.Name)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	// extractArchive 内部 safeJoin 拒绝 ../ 与绝对路径，符号链接跳过
	if err := extractArchive(tmpPath, dest); err != nil {
		return Coded(ErrCodeSkillInstallFailed, err)
	}
	return nil
}
