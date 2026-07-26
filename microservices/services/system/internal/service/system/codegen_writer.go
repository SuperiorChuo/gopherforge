package system

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	ErrRepositoryConflict = errors.New("仓库内容冲突")
	ErrRepositoryLocked   = errors.New("代码生成器写入锁已被占用")
	ErrPathOutsideRoot    = errors.New("路径超出仓库根目录")
	repositoryWriteMu     sync.Mutex
)

type WriteResult struct {
	Digest  string   `json:"digest"`
	Created []string `json:"created"`
	Patched []string `json:"patched"`
}

type RepositoryWriter struct {
	root   string
	rename func(string, string) error
}

type preparedArtifact struct {
	artifact PlannedArtifact
	target   string
	staged   string
	backup   string
	mode     fs.FileMode
}

type appliedArtifact struct {
	preparedArtifact
	createdDirectories []string
}

func NewRepositoryWriter(root string) (*RepositoryWriter, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("仓库根目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析仓库根目录失败: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("解析仓库根目录失败: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, fmt.Errorf("读取仓库根目录失败: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("仓库根目录不是目录")
	}
	return &RepositoryWriter{root: filepath.Clean(canonical), rename: os.Rename}, nil
}

func (w *RepositoryWriter) ReadFile(relative string) ([]byte, error) {
	full, err := w.resolve(relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrPathOutsideRoot, relative)
	}
	return os.ReadFile(full)
}

func (w *RepositoryWriter) ListFiles(prefix string) ([]string, error) {
	full, err := w.resolve(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	err = filepath.WalkDir(full, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(w.root, current)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (w *RepositoryWriter) Write(plan GenerationPlan) (WriteResult, error) {
	if err := validateReadyPlan(plan); err != nil {
		return WriteResult{}, err
	}
	repositoryWriteMu.Lock()
	defer repositoryWriteMu.Unlock()

	lock, err := w.acquireLock(plan.Digest)
	if err != nil {
		return WriteResult{}, err
	}
	defer func() {
		_ = lock.Close()
		_ = os.Remove(filepath.Join(w.root, ".codegen.lock"))
	}()

	artifacts := append([]PlannedArtifact(nil), plan.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	prepared, err := w.preflight(artifacts)
	if err != nil {
		return WriteResult{}, err
	}
	stageRoot, err := w.stage(plan.Digest, prepared)
	if err != nil {
		return WriteResult{}, err
	}
	defer func() {
		_ = os.RemoveAll(stageRoot)
		_ = os.Remove(filepath.Dir(stageRoot))
	}()

	result := WriteResult{Digest: plan.Digest}
	applied := make([]appliedArtifact, 0, len(prepared))
	for _, item := range prepared {
		directories, err := w.ensureParentDirectories(item.target)
		if err == nil {
			err = w.recheck(item)
		}
		if err == nil {
			err = w.rename(item.staged, item.target)
		}
		if err != nil {
			w.rollback(applied)
			for index := len(directories) - 1; index >= 0; index-- {
				_ = os.Remove(directories[index])
			}
			return WriteResult{}, fmt.Errorf("写入 %s 失败: %w", item.artifact.Path, err)
		}
		applied = append(applied, appliedArtifact{preparedArtifact: item, createdDirectories: directories})
		if item.artifact.Operation == ArtifactCreate {
			result.Created = append(result.Created, item.artifact.Path)
		} else {
			result.Patched = append(result.Patched, item.artifact.Path)
		}
	}
	return result, nil
}

func (w *RepositoryWriter) acquireLock(digest string) (*os.File, error) {
	path := filepath.Join(w.root, ".codegen.lock")
	lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, ErrRepositoryLocked
	}
	if err != nil {
		return nil, fmt.Errorf("创建代码生成器写入锁失败: %w", err)
	}
	if _, err := lock.WriteString(digest + "\n"); err != nil {
		_ = lock.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("写入代码生成器锁失败: %w", err)
	}
	return lock, nil
}

func (w *RepositoryWriter) preflight(artifacts []PlannedArtifact) ([]preparedArtifact, error) {
	prepared := make([]preparedArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		target, err := w.resolve(artifact.Path)
		if err != nil {
			return nil, err
		}
		item := preparedArtifact{artifact: artifact, target: target, mode: 0o644}
		info, statErr := os.Lstat(target)
		switch artifact.Operation {
		case ArtifactCreate:
			if statErr == nil {
				return nil, fmt.Errorf("%w: 目标文件 %s 已存在", ErrRepositoryConflict, artifact.Path)
			}
			if !errors.Is(statErr, os.ErrNotExist) {
				return nil, fmt.Errorf("检查目标文件 %s 失败: %w", artifact.Path, statErr)
			}
		case ArtifactPatch:
			if statErr != nil {
				return nil, fmt.Errorf("%w: 补丁目标 %s 不可用", ErrRepositoryConflict, artifact.Path)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%w: 补丁目标 %s 不是普通文件", ErrPathOutsideRoot, artifact.Path)
			}
			original, err := os.ReadFile(target)
			if err != nil {
				return nil, fmt.Errorf("读取补丁目标 %s 失败: %w", artifact.Path, err)
			}
			if contentHashBytes(original) != artifact.ExpectedHash {
				return nil, fmt.Errorf("%w: 补丁目标 %s 已变化", ErrRepositoryConflict, artifact.Path)
			}
			item.mode = info.Mode().Perm()
		default:
			return nil, fmt.Errorf("%w: 未知产物操作", ErrInvalidGenerationPlan)
		}
		prepared = append(prepared, item)
	}
	return prepared, nil
}

func (w *RepositoryWriter) stage(digest string, prepared []preparedArtifact) (string, error) {
	base := filepath.Join(w.root, ".codegen-tmp")
	if info, err := os.Lstat(base); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("%w: 暂存目录不可用", ErrPathOutsideRoot)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(base, 0o700); err != nil {
			return "", fmt.Errorf("创建暂存根目录失败: %w", err)
		}
	} else {
		return "", fmt.Errorf("检查暂存根目录失败: %w", err)
	}
	stageRoot := filepath.Join(base, digest)
	if err := os.Mkdir(stageRoot, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", ErrRepositoryConflict
		}
		return "", fmt.Errorf("创建暂存目录失败: %w", err)
	}
	for index := range prepared {
		prepared[index].staged = filepath.Join(stageRoot, fmt.Sprintf("content-%04d", index))
		if err := os.WriteFile(prepared[index].staged, []byte(prepared[index].artifact.Content), prepared[index].mode); err != nil {
			_ = os.RemoveAll(stageRoot)
			return "", fmt.Errorf("暂存 %s 失败: %w", prepared[index].artifact.Path, err)
		}
		if prepared[index].artifact.Operation == ArtifactPatch {
			original, err := os.ReadFile(prepared[index].target)
			if err != nil {
				_ = os.RemoveAll(stageRoot)
				return "", fmt.Errorf("备份 %s 失败: %w", prepared[index].artifact.Path, err)
			}
			prepared[index].backup = filepath.Join(stageRoot, fmt.Sprintf("backup-%04d", index))
			if err := os.WriteFile(prepared[index].backup, original, prepared[index].mode); err != nil {
				_ = os.RemoveAll(stageRoot)
				return "", fmt.Errorf("备份 %s 失败: %w", prepared[index].artifact.Path, err)
			}
		}
	}
	return stageRoot, nil
}

func (w *RepositoryWriter) recheck(item preparedArtifact) error {
	if _, err := w.resolve(item.artifact.Path); err != nil {
		return err
	}
	info, err := os.Lstat(item.target)
	if item.artifact.Operation == ArtifactCreate {
		if err == nil {
			return fmt.Errorf("%w: 目标文件已出现", ErrRepositoryConflict)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: 补丁目标不可用", ErrRepositoryConflict)
	}
	content, err := os.ReadFile(item.target)
	if err != nil {
		return err
	}
	if contentHashBytes(content) != item.artifact.ExpectedHash {
		return fmt.Errorf("%w: 补丁目标已变化", ErrRepositoryConflict)
	}
	return nil
}

func (w *RepositoryWriter) rollback(applied []appliedArtifact) {
	for index := len(applied) - 1; index >= 0; index-- {
		item := applied[index]
		if item.artifact.Operation == ArtifactCreate {
			_ = os.Remove(item.target)
		} else {
			_ = os.Rename(item.backup, item.target)
		}
		for directoryIndex := len(item.createdDirectories) - 1; directoryIndex >= 0; directoryIndex-- {
			_ = os.Remove(item.createdDirectories[directoryIndex])
		}
	}
}

func (w *RepositoryWriter) ensureParentDirectories(target string) ([]string, error) {
	parent := filepath.Dir(target)
	relative, err := filepath.Rel(w.root, parent)
	if err != nil {
		return nil, err
	}
	if relative == "." {
		return nil, nil
	}
	current := w.root
	created := make([]string, 0)
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return created, err
			}
			created = append(created, current)
			continue
		}
		if err != nil {
			return created, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return created, ErrPathOutsideRoot
		}
	}
	return created, nil
}

func (w *RepositoryWriter) resolve(relative string) (string, error) {
	if err := validateRepositoryRelativePath(relative); err != nil {
		return "", err
	}
	full := filepath.Join(w.root, filepath.FromSlash(relative))
	if !pathWithinRoot(w.root, full) {
		return "", ErrPathOutsideRoot
	}
	if err := rejectSymlinkParents(w.root, full); err != nil {
		return "", err
	}
	return full, nil
}

func validateRepositoryRelativePath(relative string) error {
	if relative == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || strings.Contains(relative, "\\") || strings.ContainsRune(relative, 0) {
		return ErrPathOutsideRoot
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != relative {
		return ErrPathOutsideRoot
	}
	return nil
}

func pathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func rejectSymlinkParents(root, target string) error {
	parent := filepath.Dir(target)
	relative, err := filepath.Rel(root, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ErrPathOutsideRoot
	}
	current := root
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrPathOutsideRoot
		}
	}
	return nil
}
