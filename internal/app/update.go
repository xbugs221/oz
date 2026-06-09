// Package app implements update checks and safe release installation for wo and oz.
package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const updateCheckTTL = 6 * time.Hour

var githubBaseURL = "https://api.github.com"

var replaceExecutableFunc = replaceExecutable

type updateTool struct {
	Name  string
	Owner string
	Repo  string
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type updateCheckCache struct {
	CheckedAt  time.Time                    `json:"checked_at"`
	TTLSeconds int64                        `json:"ttl_seconds"`
	Tools      map[string]updateCheckResult `json:"tools"`
}

type updateCheckResult struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
}

type updateInstallResult struct {
	Tool       string
	Current    string
	Latest     string
	Updated    bool
	Skipped    string
	BackupPath string
	StagedPath string
	TargetPath string
	Pending    bool
	Rollback   string
	Err        error
}

// defaultUpdateTools defines the release repositories and asset naming inputs.
func defaultUpdateTools() []updateTool {
	return []updateTool{
		{Name: "wo", Owner: "xbugs221", Repo: "oz"},
		{Name: "oz", Owner: "xbugs221", Repo: "oz"},
	}
}

// printUpdateHint appends best-effort human status update notices.
func printUpdateHint(stdout io.Writer) {
	results, err := checkUpdates(context.Background(), defaultUpdateTools(), false)
	if err != nil {
		return
	}
	var lines []string
	for _, tool := range defaultUpdateTools() {
		result, ok := results[tool.Name]
		if !ok || !result.UpdateAvailable {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s %s -> %s：运行 wo update", tool.Name, result.Current, result.Latest))
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "更新可用：")
	for _, line := range lines {
		fmt.Fprintln(stdout, line)
	}
}

// runUpdate installs newer wo and oz binaries from the same oz release batch.
func runUpdate(stdout io.Writer) error {
	client := updateHTTPClient()
	results := make([]updateInstallResult, 0, 2)
	for _, tool := range defaultUpdateTools() {
		results = append(results, installToolUpdate(context.Background(), client, tool))
	}
	for _, result := range results {
		printInstallResult(stdout, result)
	}
	return nil
}

// checkUpdates returns current/latest versions, using cache unless forceRefresh is true.
func checkUpdates(ctx context.Context, tools []updateTool, forceRefresh bool) (map[string]updateCheckResult, error) {
	cachePath, err := updateCachePath()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if !forceRefresh {
		if cache, err := readUpdateCache(cachePath); err == nil && now.Sub(cache.CheckedAt) <= time.Duration(cache.TTLSeconds)*time.Second {
			return cache.Tools, nil
		}
	}
	results := map[string]updateCheckResult{}
	client := updateHTTPClient()
	refreshed := false
	for _, tool := range tools {
		current, err := currentToolVersion(tool.Name)
		if err != nil {
			continue
		}
		if _, ok := parseVersionParts(current); !ok {
			continue
		}
		release, err := fetchLatestRelease(ctx, client, tool)
		if err != nil {
			continue
		}
		cmp, ok := compareVersions(release.TagName, current)
		if !ok {
			continue
		}
		results[tool.Name] = updateCheckResult{Current: current, Latest: release.TagName, UpdateAvailable: cmp > 0}
		refreshed = true
	}
	if refreshed {
		cache := updateCheckCache{CheckedAt: now, TTLSeconds: int64(updateCheckTTL.Seconds()), Tools: results}
		_ = writeUpdateCache(cachePath, cache)
	}
	return results, nil
}

// fetchLatestRelease reads GitHub's latest release response through an injectable base URL.
func fetchLatestRelease(ctx context.Context, client *http.Client, tool updateTool) (releaseInfo, error) {
	base := strings.TrimRight(os.Getenv("WO_GITHUB_BASE_URL"), "/")
	if base == "" {
		base = githubBaseURL
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", base, tool.Owner, tool.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return releaseInfo{}, fmt.Errorf("GitHub 返回 %s", resp.Status)
	}
	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return releaseInfo{}, err
	}
	if release.TagName == "" {
		return releaseInfo{}, errors.New("latest release 缺少 tag")
	}
	return release, nil
}

// compareVersions compares semver-like tags and refuses dev or unparsable values.
func compareVersions(a, b string) (int, bool) {
	left, ok := parseVersionParts(a)
	if !ok {
		return 0, false
	}
	right, ok := parseVersionParts(b)
	if !ok {
		return 0, false
	}
	for i := 0; i < len(left); i++ {
		if left[i] > right[i] {
			return 1, true
		}
		if left[i] < right[i] {
			return -1, true
		}
	}
	return 0, true
}

// parseVersionParts extracts major, minor and patch from release tags.
func parseVersionParts(version string) ([3]int, bool) {
	var out [3]int
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" || strings.Contains(version, "dev") || strings.Contains(version, "dirty") {
		return out, false
	}
	version = strings.Split(version, "-")[0]
	parts := strings.Split(version, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// installToolUpdate validates, backs up and replaces one tool executable.
func installToolUpdate(ctx context.Context, client *http.Client, tool updateTool) updateInstallResult {
	result := updateInstallResult{Tool: tool.Name}
	target, err := currentToolPath(tool.Name)
	if err != nil {
		result.Err = err
		return result
	}
	result.TargetPath = target
	current, err := currentToolVersion(tool.Name)
	if err != nil {
		result.Err = err
		return result
	}
	result.Current = current
	release, err := fetchLatestRelease(ctx, client, tool)
	if err != nil {
		result.Err = err
		return result
	}
	result.Latest = release.TagName
	cmp, ok := compareVersions(release.TagName, current)
	if !ok {
		result.Skipped = "版本无法比较"
		return result
	}
	if cmp <= 0 {
		result.Skipped = "已是最新"
		return result
	}
	asset, err := selectReleaseAsset(tool.Name, release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		result.Err = err
		return result
	}
	sumAsset, err := selectChecksumAsset(release)
	if err != nil {
		result.Err = err
		return result
	}
	tempDir, err := os.MkdirTemp("", "wo-update-*")
	if err != nil {
		result.Err = err
		return result
	}
	defer os.RemoveAll(tempDir)
	archivePath, err := downloadAsset(ctx, client, asset, tempDir)
	if err != nil {
		result.Err = err
		return result
	}
	sumPath, err := downloadAsset(ctx, client, sumAsset, tempDir)
	if err != nil {
		result.Err = err
		return result
	}
	if err := verifyChecksum(archivePath, sumPath); err != nil {
		result.Err = err
		return result
	}
	staged, err := extractBinary(archivePath, tool.Name, tempDir)
	if err != nil {
		result.Err = err
		return result
	}
	if err := verifyBinaryVersion(staged, release.TagName); err != nil {
		result.Err = err
		return result
	}
	result.StagedPath = staged
	backup, err := backupExecutable(target, tool.Name, current)
	if err != nil {
		result.Err = fmt.Errorf("备份失败：%w", err)
		return result
	}
	result.BackupPath = backup
	result.Rollback = copyCommand(backup, target, runtime.GOOS)
	replaceResult, err := replaceExecutableFunc(target, staged)
	result.StagedPath = replaceResult.StagedPath
	result.Pending = replaceResult.Pending
	if err != nil {
		if kept, keepErr := preserveStagedBinary(staged, tool.Name, release.TagName); keepErr == nil {
			result.StagedPath = kept
		}
		result.Err = err
		return result
	}
	result.Updated = true
	if !result.Pending {
		result.StagedPath = ""
		if err := verifyBinaryVersion(target, release.TagName); err != nil {
			result.Updated = false
			result.Err = fmt.Errorf("安装后版本验证失败：%w", err)
			return result
		}
	}
	return result
}

// currentToolPath resolves wo through argv or PATH and oz through PATH.
func currentToolPath(name string) (string, error) {
	if name == "wo" {
		if override := os.Getenv("WO_UPDATE_SELF_PATH"); override != "" {
			return override, nil
		}
		if exe, err := os.Executable(); err == nil && exe != "" {
			return exe, nil
		}
	}
	return resolveCommand(name)
}

// currentToolVersion reads local tool versions from wo internals or --version.
func currentToolVersion(name string) (string, error) {
	if name == "wo" {
		return resolvedVersion(), nil
	}
	path, err := resolveCommand(name)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := commandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", errors.New("版本输出为空")
	}
	return fields[0], nil
}

// selectReleaseAsset chooses the platform-specific archive.
func selectReleaseAsset(tool string, release releaseInfo, goos, goarch string) (releaseAsset, error) {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	want := fmt.Sprintf("%s_%s_%s_%s%s", tool, release.TagName, goos, goarch, ext)
	for _, asset := range release.Assets {
		if asset.Name == want {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("缺少当前平台资产 %s", want)
}

// selectChecksumAsset finds sha256sums.txt in a release.
func selectChecksumAsset(release releaseInfo) (releaseAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name == "sha256sums.txt" {
			return asset, nil
		}
	}
	return releaseAsset{}, errors.New("缺少 sha256sums.txt")
}

// downloadAsset downloads one release asset to a temp directory.
func downloadAsset(ctx context.Context, client *http.Client, asset releaseAsset, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载 %s 失败：%s", asset.Name, resp.Status)
	}
	path := filepath.Join(dir, asset.Name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

// verifyChecksum checks the archive digest against sha256sums.txt.
func verifyChecksum(archivePath, sumPath string) error {
	wantName := filepath.Base(archivePath)
	data, err := os.ReadFile(sumPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == wantName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("sha256sums.txt 缺少 %s", wantName)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("校验失败：%s", wantName)
	}
	return nil
}

// extractBinary extracts the requested binary from tar.gz or zip archives.
func extractBinary(archivePath, tool, dir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipBinary(archivePath, tool, dir)
	}
	return extractTarGzBinary(archivePath, tool, dir)
}

// extractTarGzBinary extracts a release binary from a tar.gz archive.
func extractTarGzBinary(archivePath, tool, dir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != executableName(tool) {
			continue
		}
		return writeStagedBinary(reader, filepath.Join(dir, executableName(tool)))
	}
	return "", fmt.Errorf("压缩包缺少 %s", executableName(tool))
}

// extractZipBinary extracts a release binary from a zip archive.
func extractZipBinary(archivePath, tool, dir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != executableName(tool) {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		return writeStagedBinary(src, filepath.Join(dir, executableName(tool)))
	}
	return "", fmt.Errorf("压缩包缺少 %s", executableName(tool))
}

// writeStagedBinary writes an executable staged binary.
func writeStagedBinary(src io.Reader, path string) (string, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}
	return path, nil
}

// verifyBinaryVersion ensures the staged binary reports the release tag.
func verifyBinaryVersion(path, latest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := commandContext(ctx, path, "--version").Output()
	if err != nil {
		return err
	}
	got := strings.TrimSpace(string(out))
	if got != latest {
		return fmt.Errorf("版本验证失败：got %q want %q", got, latest)
	}
	return nil
}

// backupExecutable copies the current binary to user state before replacement.
func backupExecutable(target, tool, version string) (string, error) {
	root, err := runtimeRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "backups", tool)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102T150405Z") + "-" + filepath.Base(target) + "-" + strings.ReplaceAll(version, string(filepath.Separator), "_")
	backup := filepath.Join(dir, name)
	in, err := os.Open(target)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return backup, nil
}

type replaceResult struct {
	StagedPath string
	Pending    bool
}

// replaceExecutable atomically swaps the staged binary into place where supported.
func replaceExecutable(target, staged string) (replaceResult, error) {
	return replaceExecutableForGOOS(target, staged, runtime.GOOS, os.Getpid())
}

// replaceExecutableForGOOS handles platform-specific replacement behavior.
func replaceExecutableForGOOS(target, staged, goos string, pid int) (replaceResult, error) {
	result := replaceResult{StagedPath: staged}
	if goos == "windows" {
		next := target + ".new"
		if err := os.Rename(staged, next); err != nil {
			return result, err
		}
		result.StagedPath = next
		script := target + ".replace.cmd"
		body := fmt.Sprintf("@echo off\r\n:wait\r\ntasklist /FI \"PID eq %d\" | find \"%d\" >nul\r\nif not errorlevel 1 (\r\n  timeout /T 1 /NOBREAK >nul\r\n  goto wait\r\n)\r\nmove /Y %q %q\r\n", pid, pid, next, target)
		if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
			return result, err
		}
		if err := exec.Command("cmd", "/C", "start", "", "/B", script).Start(); err != nil {
			return result, err
		}
		result.Pending = true
		return result, nil
	}
	info, err := os.Stat(target)
	if err != nil {
		return result, err
	}
	if err := os.Chmod(staged, info.Mode().Perm()); err != nil {
		return result, err
	}
	return result, os.Rename(staged, target)
}

// preserveStagedBinary moves a replace-failed binary to user state for manual handling.
func preserveStagedBinary(staged, tool, version string) (string, error) {
	root, err := runtimeRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "pending-updates", tool)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102T150405Z") + "-" + executableName(tool) + "-" + strings.ReplaceAll(version, string(filepath.Separator), "_")
	kept := filepath.Join(dir, name)
	return kept, os.Rename(staged, kept)
}

// printInstallResult writes one tool's update outcome.
func printInstallResult(stdout io.Writer, result updateInstallResult) {
	if result.Err != nil {
		fmt.Fprintf(stdout, "%s: 更新失败：%v\n", result.Tool, result.Err)
		if result.BackupPath != "" {
			fmt.Fprintf(stdout, "  备份: %s\n", result.BackupPath)
			rollback := result.Rollback
			if rollback == "" {
				rollback = copyCommand(result.BackupPath, result.TargetPath, runtime.GOOS)
			}
			fmt.Fprintf(stdout, "  回滚: %s\n", rollback)
		}
		if result.StagedPath != "" {
			fmt.Fprintf(stdout, "  已保留新版: %s\n", result.StagedPath)
			fmt.Fprintf(stdout, "  手动替换: %s\n", copyCommand(result.StagedPath, result.TargetPath, runtime.GOOS))
		}
		return
	}
	if result.Skipped != "" {
		if result.Latest != "" {
			fmt.Fprintf(stdout, "%s: %s %s\n", result.Tool, result.Skipped, result.Latest)
		} else {
			fmt.Fprintf(stdout, "%s: %s\n", result.Tool, result.Skipped)
		}
		return
	}
	if result.Updated {
		if result.Pending {
			fmt.Fprintf(stdout, "%s: 已下载 %s -> %s，更新将在当前进程退出后完成\n", result.Tool, result.Current, result.Latest)
		} else {
			fmt.Fprintf(stdout, "%s: 已更新 %s -> %s\n", result.Tool, result.Current, result.Latest)
		}
		fmt.Fprintf(stdout, "  备份: %s\n", result.BackupPath)
		fmt.Fprintf(stdout, "  回滚: %s\n", result.Rollback)
	}
}

// copyCommand renders a platform executable copy command for rollback guidance.
func copyCommand(src, dst, goos string) string {
	if goos == "windows" {
		return fmt.Sprintf("powershell -NoProfile -Command \"Copy-Item -LiteralPath %s -Destination %s -Force\"", powershellQuote(src), powershellQuote(dst))
	}
	return fmt.Sprintf("cp %s %s", shellQuote(src), shellQuote(dst))
}

// shellQuote quotes one POSIX shell argument.
func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// powershellQuote quotes one PowerShell string literal.
func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// updateHTTPClient returns a short-timeout client for status checks and update downloads.
func updateHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

// updateCachePath resolves the user cache file for update checks.
func updateCachePath() (string, error) {
	if override := os.Getenv("WO_UPDATE_CACHE"); override != "" {
		return override, nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "wo", "update-check.json"), nil
}

// readUpdateCache decodes update check cache.
func readUpdateCache(path string) (updateCheckCache, error) {
	var cache updateCheckCache
	data, err := os.ReadFile(path)
	if err != nil {
		return cache, err
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return cache, err
	}
	if cache.TTLSeconds <= 0 || cache.Tools == nil {
		return cache, errors.New("缓存无效")
	}
	return cache, nil
}

// writeUpdateCache persists update check cache.
func writeUpdateCache(path string, cache updateCheckCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// executableName returns the platform binary file name.
func executableName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}
