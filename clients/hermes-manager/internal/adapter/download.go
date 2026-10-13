package adapter

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// uv 引导：版本与各平台 sha256 钉死在源码里，下载后校验通过才落盘执行。
// 升级 uv = 改这里的常量 + 发新 manager 版本，不存在运行时跟随 latest。
const uvVersion = "0.8.4"

var uvChecksums = map[string]struct {
	url    string
	sha256 string
	// zip 内条目名（tar.gz 解出目录，zip 直接是 uv.exe）
	archived string
}{
	"darwin/arm64": {
		url:      "https://github.com/astral-sh/uv/releases/download/" + uvVersion + "/uv-aarch64-apple-darwin.tar.gz",
		sha256:   "ef6785df8c23232ce6209c04acefd0c0d2ffb3a3ba0eef16422bdfe99a059105",
		archived: "uv-aarch64-apple-darwin/uv",
	},
	"darwin/amd64": {
		url:      "https://github.com/astral-sh/uv/releases/download/" + uvVersion + "/uv-x86_64-apple-darwin.tar.gz",
		sha256:   "14e5309f182d1a92cf6c82f5891a0a0dc1cd5d46627171eaa1e84fa2b7e0afc3",
		archived: "uv-x86_64-apple-darwin/uv",
	},
	"windows/amd64": {
		url:      "https://github.com/astral-sh/uv/releases/download/" + uvVersion + "/uv-x86_64-pc-windows-msvc.zip",
		sha256:   "817c50c80229f88de9699626ee3774c0cceed86099663e8fb00c5ffae7ea911c",
		archived: "uv.exe",
	},
}

const (
	// 制品与 uv 的下载大小上限。上游源码 tar 约 30MB，uv 约 20MB；
	// 512MB 是防挂错链接的护栏，不是精确预算。
	maxDownloadBytes = 512 << 20
	// 安装一版所需的最低可用空间（源码 + venv + 依赖，宽估）。
	minFreeBytes = 3 << 30
)

func uvBinaryName() string {
	if runtime.GOOS == "windows" {
		return "uv.exe"
	}
	return "uv"
}

// ensureUV 确保 tools/uv 可用；不存在则按钉死的 URL+校验和引导下载。
func (b *Base) ensureUV(ctx context.Context) (string, error) {
	uvPath := filepath.Join(b.home, "tools", uvBinaryName())
	if _, err := os.Stat(uvPath); err == nil {
		return uvPath, nil
	}
	spec, ok := uvChecksums[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return "", Coded(ErrCodeBootstrapFailed, fmt.Errorf("no pinned uv for %s/%s", runtime.GOOS, runtime.GOARCH))
	}
	if err := os.MkdirAll(filepath.Join(b.home, "tools"), 0o700); err != nil {
		return "", Coded(ErrCodeBootstrapFailed, err)
	}

	archivePath, err := b.download(ctx, spec.url, spec.sha256, "uv-download-*")
	if err != nil {
		return "", Coded(ErrCodeBootstrapFailed, err)
	}
	defer os.Remove(archivePath)

	if err := extractSingleFile(archivePath, spec.archived, uvPath); err != nil {
		return "", Coded(ErrCodeBootstrapFailed, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(uvPath, 0o755); err != nil {
			return "", Coded(ErrCodeBootstrapFailed, err)
		}
	}
	return uvPath, nil
}

// download 下载到临时文件并强制校验 sha256；不符即删。
// https 强制；http 仅当 AllowInsecureHTTP（内网自托管制品仓场景）显式打开。
func (b *Base) download(ctx context.Context, rawURL, wantSHA256, pattern string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid artifact URL: %w", err)
	}
	if err := validateDownloadURL(parsedURL, b.allowInsecureHTTP); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", err
	}
	client := *b.httpClient
	originalCheckRedirect := client.CheckRedirect
	initialScheme := strings.ToLower(parsedURL.Scheme)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := validateDownloadURL(request.URL, b.allowInsecureHTTP); err != nil {
			return err
		}
		if initialScheme == "https" && strings.ToLower(request.URL.Scheme) != "https" {
			return errors.New("HTTPS artifact download may not redirect to an insecure scheme")
		}
		if originalCheckRedirect != nil {
			return originalCheckRedirect(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", rawURL, response.StatusCode)
	}

	temporary, err := os.CreateTemp(b.home, pattern)
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(response.Body, maxDownloadBytes+1))
	if err != nil {
		cleanup()
		return "", err
	}
	if written > maxDownloadBytes {
		cleanup()
		return "", fmt.Errorf("download exceeds %d bytes", int64(maxDownloadBytes))
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return "", err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, wantSHA256) {
		_ = os.Remove(temporaryName)
		return "", fmt.Errorf("sha256 mismatch: got %s want %s", got, wantSHA256)
	}
	return temporaryName, nil
}

func validateDownloadURL(target *url.URL, allowInsecureHTTP bool) error {
	if target == nil {
		return errors.New("artifact URL is required")
	}
	switch strings.ToLower(target.Scheme) {
	case "https":
	case "http":
		if !allowInsecureHTTP {
			return errors.New("artifact URL must use https")
		}
	default:
		return errors.New("artifact URL must use http or https")
	}
	if target.Host == "" {
		return errors.New("artifact URL must be absolute and include a host")
	}
	return nil
}

// extractArchive 把 tar.gz / zip 全量解压到 destDir，剥掉顶层目录
// （GitHub 源码包形如 repo-<sha>/...）。防路径穿越：拒绝绝对路径与 ..。
func extractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".zip") || isZip(archivePath) {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func isZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 2)
	if _, err := io.ReadFull(f, magic); err != nil {
		return false
	}
	return magic[0] == 'P' && magic[1] == 'K'
}

// safeJoin 拼接并拒绝逃出 destDir 的条目（../、绝对路径、盘符）。
func safeJoin(destDir, name string) (string, error) {
	name = filepath.ToSlash(name)
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe archive entry: %q", name)
	}
	joined := filepath.Join(destDir, filepath.FromSlash(name))
	if !strings.HasPrefix(joined, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %q", name)
	}
	return joined, nil
}

// stripTop 去掉条目的顶层目录段；条目本身就是顶层目录时返回空。
func stripTop(name string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	if index := strings.IndexByte(name, '/'); index >= 0 {
		return name[index+1:]
	}
	return ""
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := stripTop(header.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(destDir, name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(reader, maxDownloadBytes)); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// 符号链接等一律跳过：源码包不需要，且 symlink 是穿越向量
		}
	}
}

func extractZip(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		name := stripTop(file.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(destDir, name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyZipEntry(file, target); err != nil {
			return err
		}
	}
	return nil
}

func copyZipEntry(file *zip.File, target string) error {
	in, err := file.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	mode := file.Mode() & 0o777
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, maxDownloadBytes)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// extractSingleFile 从归档中取一个指定条目（uv 引导用）。
func extractSingleFile(archivePath, entryName, destPath string) error {
	tempDir, err := os.MkdirTemp(filepath.Dir(destPath), "extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	// zip（uv.exe 平铺）不能剥顶层，tar.gz（uv-<target>/uv）要剥——
	// 统一策略：全量解到临时目录，再按剥层后的名字取。
	if err := extractArchive(archivePath, tempDir); err != nil {
		return err
	}
	candidate := filepath.Join(tempDir, filepath.FromSlash(stripTopOrSelf(entryName)))
	data, err := os.ReadFile(candidate)
	if err != nil {
		return fmt.Errorf("archive entry %s: %w", entryName, err)
	}
	return atomicWriteFile(filepath.Dir(destPath), filepath.Base(destPath), data, 0o755)
}

func stripTopOrSelf(name string) string {
	if stripped := stripTop(name); stripped != "" {
		return stripped
	}
	return name
}
