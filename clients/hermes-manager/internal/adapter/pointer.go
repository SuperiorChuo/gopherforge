package adapter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// currentPointer 是活跃版本指针（home/current.json）。
//
// 用指针文件而不用符号链接：Windows 普通用户建不了 symlink，junction 语义
// 又不同；JSON 文件跨平台一致，且可借 state.Store 同款原子写。
type currentPointer struct {
	Version    string `json:"version"`
	ArtifactID uint64 `json:"artifact_id"`
	SHA256     string `json:"sha256"`
	// PreviousVersion 用于回滚：切指针前把旧值挪进来。只保留一级——
	// 文档 §9.3 的回滚目标就是「上一版本」，不做多级历史。
	PreviousVersion string `json:"previous_version,omitempty"`
	// PreviousProfileBackup 是 backups/ 下由 manager 自己创建的
	// 直接子目录名（不是任意路径），用于回滚更新前的受管配置。
	PreviousProfileBackup string `json:"previous_profile_backup,omitempty"`
}

func (b *Base) loadPointer() (*currentPointer, error) {
	data, err := os.ReadFile(filepath.Join(b.home, "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // 未安装不是错误
	}
	if err != nil {
		return nil, err
	}
	var pointer currentPointer
	if err := json.Unmarshal(data, &pointer); err != nil {
		return nil, err
	}
	if !safeArtifactVersion(pointer.Version) ||
		(pointer.PreviousVersion != "" && !safeArtifactVersion(pointer.PreviousVersion)) {
		return nil, errors.New("current pointer contains an unsafe artifact version")
	}
	return &pointer, nil
}

func (b *Base) savePointer(pointer *currentPointer) error {
	if pointer == nil {
		return errors.New("current pointer is nil")
	}
	if !safeArtifactVersion(pointer.Version) ||
		(pointer.PreviousVersion != "" && !safeArtifactVersion(pointer.PreviousVersion)) {
		return errors.New("artifact version must be a safe directory name")
	}
	if !safeProfileBackupName(pointer.PreviousProfileBackup) {
		return errors.New("profile backup must be a direct child of the backups directory")
	}
	data, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(b.home, "current.json", append(data, '\n'), 0o600)
}

// safeArtifactVersion 同时是目录边界。版本会直接成为 versions/ 的子目录名，
// 因此只允许发布标签常用的 ASCII 字符，并拒绝空值、路径分隔符和点目录。
func safeArtifactVersion(version string) bool {
	if len(version) == 0 || len(version) > 40 {
		return false
	}
	isAlphaNum := func(char byte) bool {
		return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
	}
	if !isAlphaNum(version[0]) || !isAlphaNum(version[len(version)-1]) {
		return false
	}
	for _, char := range []byte(version) {
		if !isAlphaNum(char) && char != '.' && char != '_' && char != '+' && char != '-' {
			return false
		}
	}
	return true
}

// safeProfileBackupName 只接受 backups/ 下的一层目录名。指针文件可能被
// 本机用户篡改，Rollback 不能因此去读取任意路径。空值用于兼容旧指针。
func safeProfileBackupName(name string) bool {
	if name == "" {
		return true
	}
	clean := filepath.Clean(name)
	return clean != "." && clean != ".." && !filepath.IsAbs(name) && clean == filepath.Base(clean)
}

// atomicWriteFile 与 state.Store.atomicWrite 同构：临时文件 + fsync + rename。
// adapter 不依赖 state 包（避免反向耦合），这里复刻一份。
func atomicWriteFile(dir, name string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(dir, ".hermes-adapter-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(dir, name))
}
