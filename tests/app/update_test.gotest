// Package app tests the user-visible update check and install behavior.
package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCompareVersionsHandlesReleaseTags verifies updater ordering ignores a leading v.
func TestCompareVersionsHandlesReleaseTags(t *testing.T) {
	cmp, ok := compareVersions("v1.2.4", "1.2.3")
	if !ok || cmp <= 0 {
		t.Fatalf("compareVersions = %d/%v, want newer", cmp, ok)
	}
	if _, ok := compareVersions("dev", "v1.2.3"); ok {
		t.Fatal("dev should not be comparable")
	}
}

// setTestGithubBaseURL points release API calls at a per-test server.
func setTestGithubBaseURL(t *testing.T, rawURL string) {
	t.Helper()
	previous := githubBaseURL
	githubBaseURL = rawURL
	t.Cleanup(func() { githubBaseURL = previous })
}

// testUpdateAssetTrust returns a download trust policy scoped to one httptest server.
func testUpdateAssetTrust(t *testing.T, rawURL string) updateAssetTrustPolicy {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.ToLower(parsed.Hostname())
	trust := defaultUpdateAssetTrustPolicy()
	if host != "" {
		trust.TrustedHosts[host] = true
	}
	return trust
}

// TestUpdateCheckUsesFreshCacheWithoutNetwork verifies status can show cached updates offline.
func TestUpdateCheckUsesFreshCacheWithoutNetwork(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	t.Setenv("WO_UPDATE_CACHE", cachePath)
	setTestGithubBaseURL(t, "http://127.0.0.1:1")
	cache := updateCheckCache{
		CheckedAt:  time.Now().UTC(),
		TTLSeconds: int64(updateCheckTTL.Seconds()),
		Tools: map[string]updateCheckResult{
			"wo": {Current: "v1.0.0", Latest: "v1.1.0", UpdateAvailable: true},
		},
	}
	if err := writeUpdateCache(cachePath, cache); err != nil {
		t.Fatal(err)
	}
	results, err := checkUpdates(t.Context(), defaultUpdateTools(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !results["wo"].UpdateAvailable {
		t.Fatalf("cache result = %#v, want update", results)
	}
}

// TestDefaultUpdateToolsUseOzReleaseBatch verifies wo and oz share one release source.
func TestDefaultUpdateToolsUseOzReleaseBatch(t *testing.T) {
	tools := defaultUpdateTools()
	if len(tools) != 2 {
		t.Fatalf("default tools = %#v", tools)
	}
	for _, tool := range tools {
		if tool.Owner != "xbugs221" || tool.Repo != "oz" {
			t.Fatalf("tool %s uses split release source: %#v", tool.Name, tool)
		}
	}
}

// TestExpiredCacheRefreshFailureKeepsOldCache verifies failed refreshes do not poison cache.
func TestExpiredCacheRefreshFailureKeepsOldCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	t.Setenv("WO_UPDATE_CACHE", cachePath)
	setTestGithubBaseURL(t, "http://127.0.0.1:1")
	cache := updateCheckCache{
		CheckedAt:  time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		TTLSeconds: int64(updateCheckTTL.Seconds()),
		Tools: map[string]updateCheckResult{
			"wo": {Current: "v1.0.0", Latest: "v1.1.0", UpdateAvailable: true},
		},
	}
	if err := writeUpdateCache(cachePath, cache); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	if _, err := checkUpdates(t.Context(), []updateTool{{Name: "wo", Owner: "xbugs221", Repo: "wo"}}, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failed refresh rewrote cache:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestFetchLatestReleaseIgnoresEnvironmentBaseURL verifies env cannot redirect production trust.
func TestFetchLatestReleaseIgnoresEnvironmentBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/xbugs221/oz/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v1.2.3","assets":[]}`)
	}))
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)
	t.Setenv("WO_GITHUB_BASE_URL", "http://127.0.0.1:1")
	release, err := fetchLatestRelease(t.Context(), server.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "oz"})
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v1.2.3" {
		t.Fatalf("release = %#v, want test server release", release)
	}
}

// TestCheckUpdatesSkipsEmptyOzVersion verifies malformed local versions stay best-effort.
func TestCheckUpdatesSkipsEmptyOzVersion(t *testing.T) {
	dir := t.TempDir()
	ozPath := filepath.Join(dir, "oz")
	if err := os.WriteFile(ozPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WO_UPDATE_CACHE", filepath.Join(t.TempDir(), "cache.json"))
	setTestGithubBaseURL(t, "http://127.0.0.1:1")
	if _, err := checkUpdates(t.Context(), []updateTool{{Name: "oz", Owner: "xbugs221", Repo: "oz"}}, true); err != nil {
		t.Fatalf("empty oz version should be skipped, got %v", err)
	}
}

// TestRunUpdateWorksOutsideGitRepo verifies global maintenance commands do not need a repo.
func TestRunUpdateWorksOutsideGitRepo(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	target := filepath.Join(t.TempDir(), "wo")
	if err := os.WriteFile(target, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", target)
	t.Setenv("PATH", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/xbugs221/oz/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)
	chdir(t, t.TempDir())

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"update"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("update outside git repo failed: %v, stderr=%q", err, stderr.String())
	}
	got := stdout.String()
	if strings.Contains(got, "当前目录不在 git 仓库内") {
		t.Fatalf("update should not require git repo:\n%s", got)
	}
	for _, want := range []string{"wo: 已是最新 v1.0.0", "oz: 更新失败"} {
		if !strings.Contains(got, want) {
			t.Fatalf("update output missing %q:\n%s", want, got)
		}
	}
}

// TestSelectReleaseAssetChoosesPlatformArchive verifies release asset naming is strict.
func TestSelectReleaseAssetChoosesPlatformArchive(t *testing.T) {
	release := releaseInfo{TagName: "v1.2.3", Assets: []releaseAsset{{Name: "wo_v1.2.3_linux_amd64.tar.gz"}}}
	asset, err := selectReleaseAsset("wo", release, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "wo_v1.2.3_linux_amd64.tar.gz" {
		t.Fatalf("asset = %s", asset.Name)
	}
}

// TestVerifyChecksumRejectsMismatch verifies bad downloads cannot be installed.
func TestVerifyChecksumRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "wo_v1.0.0_linux_amd64.tar.gz")
	if err := os.WriteFile(archive, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(dir, "sha256sums.txt")
	if err := os.WriteFile(sums, []byte(strings.Repeat("0", 64)+"  "+filepath.Base(archive)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archive, sums); err == nil {
		t.Fatal("checksum mismatch should fail")
	}
}

// TestDownloadAssetRejectsOversizedChecksum verifies update downloads are bounded.
func TestDownloadAssetRejectsOversizedChecksum(t *testing.T) {
	body := strings.Repeat("x", maxChecksumFileBytes+1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)
	_, err := downloadAssetWithTrust(t.Context(), server.Client(), releaseAsset{Name: "sha256sums.txt", BrowserDownloadURL: server.URL}, t.TempDir(), testUpdateAssetTrust(t, server.URL))
	if err == nil || !strings.Contains(err.Error(), "超过大小上限") {
		t.Fatalf("downloadAsset err = %v, want size limit", err)
	}
}

// TestDownloadAssetRejectsHTTPOnConfiguredAPIHost verifies test API hosts do not bypass production URL policy.
func TestDownloadAssetRejectsHTTPOnConfiguredAPIHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not used")
	}))
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)

	_, err := downloadAssetWithTrust(t.Context(), server.Client(), releaseAsset{Name: "wo.tgz", BrowserDownloadURL: server.URL + "/wo.tgz"}, t.TempDir(), testUpdateAssetTrust(t, server.URL))
	if err == nil || !strings.Contains(err.Error(), "必须使用 https") {
		t.Fatalf("downloadAsset err = %v, want https rejection", err)
	}
}

// TestDownloadAssetRejectsUntrustedURL verifies release assets must come from trusted HTTPS hosts.
func TestDownloadAssetRejectsUntrustedURL(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/xbugs221/oz/releases/download/v1.0.0/wo.tgz",
		"file:///tmp/wo.tgz",
		"https://example.com/wo.tgz",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := downloadAsset(t.Context(), http.DefaultClient, releaseAsset{Name: "wo.tgz", BrowserDownloadURL: raw}, t.TempDir())
			if err == nil {
				t.Fatalf("downloadAsset(%q) succeeded, want trusted URL error", raw)
			}
		})
	}
}

// TestVerifyBinaryVersionRejectsWrongTag verifies staged binaries must match latest.
func TestVerifyBinaryVersionRejectsWrongTag(t *testing.T) {
	for _, output := range []string{"v1.0.0", "v1.1.0-dev", "not v1.1.0"} {
		bin := filepath.Join(t.TempDir(), "wo")
		if err := os.WriteFile(bin, fakeBinary(output), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := verifyBinaryVersion(bin, "v1.1.0"); err == nil {
			t.Fatalf("staged version %q should fail", output)
		}
	}
}

// TestBackupExecutableFailsBeforeReplacement verifies missing old binaries are not replaced.
func TestBackupExecutableFailsBeforeReplacement(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := backupExecutable(filepath.Join(t.TempDir(), "missing-wo"), "wo", "v1.0.0"); err == nil {
		t.Fatal("backup should fail when target binary is missing")
	}
}

// TestInstallToolUpdateBackupFailureKeepsOriginal verifies backup failures stop replacement.
func TestInstallToolUpdateBackupFailureKeepsOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	stateHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stateHome, "wo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateHome, "wo", "backups"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", stateHome)
	target := filepath.Join(t.TempDir(), "wo")
	oldBytes := fakeBinary("v1.0.0")
	if err := os.WriteFile(target, oldBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", target)
	server, _ := updateServer(t, "wo", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)

	result := installToolUpdateWithTrust(t.Context(), server.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "wo"}, testUpdateAssetTrust(t, server.URL))
	if result.Err == nil || !strings.Contains(result.Err.Error(), "备份失败") {
		t.Fatalf("result = %#v, want backup failure", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, oldBytes) {
		t.Fatalf("backup failure changed target:\n%s", data)
	}
}

// TestInstallToolUpdateBacksUpReplacesAndPrintsRollback verifies the real update path.
func TestInstallToolUpdateBacksUpReplacesAndPrintsRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	target := filepath.Join(t.TempDir(), "wo")
	if err := os.WriteFile(target, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", target)

	server, _ := updateServer(t, "wo", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)

	result := installToolUpdateWithTrust(t.Context(), server.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "wo"}, testUpdateAssetTrust(t, server.URL))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !result.Updated || result.BackupPath == "" || result.Rollback == "" {
		t.Fatalf("result = %#v, want updated with backup", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("v1.1.0")) {
		t.Fatalf("target was not replaced: %q", string(data))
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

// TestInstallToolUpdateCanUpdateOz verifies downstream oz uses PATH and the same safe flow.
func TestInstallToolUpdateCanUpdateOz(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "oz")
	if err := os.WriteFile(target, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	server, _ := updateServer(t, "oz", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)

	result := installToolUpdateWithTrust(t.Context(), server.Client(), updateTool{Name: "oz", Owner: "xbugs221", Repo: "oz"}, testUpdateAssetTrust(t, server.URL))
	if result.Err != nil || !result.Updated || result.BackupPath == "" {
		t.Fatalf("result = %#v", result)
	}
}

// TestOneToolFailureDoesNotRollbackSuccessfulTool verifies independent update outcomes.
func TestOneToolFailureDoesNotRollbackSuccessfulTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	woTarget := filepath.Join(t.TempDir(), "wo")
	if err := os.WriteFile(woTarget, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", woTarget)
	woServer, _ := updateServer(t, "wo", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer woServer.Close()
	setTestGithubBaseURL(t, woServer.URL)
	woResult := installToolUpdateWithTrust(t.Context(), woServer.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "wo"}, testUpdateAssetTrust(t, woServer.URL))
	if woResult.Err != nil || !woResult.Updated {
		t.Fatalf("wo result = %#v", woResult)
	}

	ozDir := t.TempDir()
	ozTarget := filepath.Join(ozDir, "oz")
	if err := os.WriteFile(ozTarget, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ozDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ozServer, _ := updateServer(t, "oz", "v1.1.0", fakeBinary("v1.1.0"), true)
	defer ozServer.Close()
	setTestGithubBaseURL(t, ozServer.URL)
	ozResult := installToolUpdateWithTrust(t.Context(), ozServer.Client(), updateTool{Name: "oz", Owner: "xbugs221", Repo: "oz"}, testUpdateAssetTrust(t, ozServer.URL))
	if ozResult.Err == nil || !strings.Contains(ozResult.Err.Error(), "校验失败") {
		t.Fatalf("oz result = %#v, want checksum failure", ozResult)
	}
	data, err := os.ReadFile(woTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("v1.1.0")) {
		t.Fatalf("successful wo update was rolled back: %q", string(data))
	}
	ozData, err := os.ReadFile(ozTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ozData, []byte("v1.0.0")) {
		t.Fatalf("failed oz update changed original: %q", string(ozData))
	}
}

// TestReplaceFailureKeepsBackupAndStagedPath verifies manual recovery data is printed.
func TestReplaceFailureKeepsBackupAndStagedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	target := filepath.Join(t.TempDir(), "wo")
	if err := os.WriteFile(target, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", target)
	server, _ := updateServer(t, "wo", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)
	oldReplace := replaceExecutableFunc
	replaceExecutableFunc = func(target, staged string) (replaceResult, error) {
		return replaceResult{StagedPath: staged}, fmt.Errorf("replace denied")
	}
	t.Cleanup(func() { replaceExecutableFunc = oldReplace })

	result := installToolUpdateWithTrust(t.Context(), server.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "wo"}, testUpdateAssetTrust(t, server.URL))
	if result.Err == nil || result.BackupPath == "" || result.StagedPath == "" {
		t.Fatalf("result = %#v, want failure with backup and staged paths", result)
	}
	if _, err := os.Stat(result.StagedPath); err != nil {
		t.Fatalf("staged binary was not preserved: %v", err)
	}
	var out bytes.Buffer
	printInstallResult(&out, result)
	for _, want := range []string{"备份:", "回滚:", "已保留新版:", "手动替换:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

// TestInstalledVersionFailureReportsRollback verifies target --version is checked after replace.
func TestInstalledVersionFailureReportsRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replace flow differs on windows")
	}
	oldVersion := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	target := filepath.Join(t.TempDir(), "wo")
	if err := os.WriteFile(target, fakeBinary("v1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WO_UPDATE_SELF_PATH", target)
	server, _ := updateServer(t, "wo", "v1.1.0", fakeBinary("v1.1.0"), false)
	defer server.Close()
	setTestGithubBaseURL(t, server.URL)
	oldReplace := replaceExecutableFunc
	replaceExecutableFunc = func(target, staged string) (replaceResult, error) {
		if err := os.WriteFile(target, fakeBinary("v9.9.9"), 0o755); err != nil {
			return replaceResult{StagedPath: staged}, err
		}
		_ = os.Remove(staged)
		return replaceResult{StagedPath: staged}, nil
	}
	t.Cleanup(func() { replaceExecutableFunc = oldReplace })

	result := installToolUpdateWithTrust(t.Context(), server.Client(), updateTool{Name: "wo", Owner: "xbugs221", Repo: "wo"}, testUpdateAssetTrust(t, server.URL))
	if result.Err == nil || !strings.Contains(result.Err.Error(), "安装后版本验证失败") {
		t.Fatalf("result = %#v, want installed version failure", result)
	}
	if result.BackupPath == "" || result.StagedPath != "" {
		t.Fatalf("result = %#v, want backup without stale staged path", result)
	}
	var out bytes.Buffer
	printInstallResult(&out, result)
	if !strings.Contains(out.String(), "回滚:") || !strings.Contains(out.String(), result.BackupPath) {
		t.Fatalf("output missing rollback guidance:\n%s", out.String())
	}
}

// TestWindowsReplaceStartsPendingScript verifies Windows self-update is scheduled.
func TestWindowsReplaceStartsPendingScript(t *testing.T) {
	target := filepath.Join(t.TempDir(), "wo.exe")
	staged := filepath.Join(t.TempDir(), "wo-staged.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := replaceExecutableForGOOS(target, staged, "windows", 12345)
	if runtime.GOOS == "windows" {
		if err != nil || !result.Pending {
			t.Fatalf("windows result = %#v err=%v", result, err)
		}
		return
	}
	if err == nil {
		t.Fatal("non-windows host should fail to start cmd during simulated windows replacement")
	}
	if _, statErr := os.Stat(target + ".replace.cmd"); statErr != nil {
		t.Fatalf("replacement script missing: %v", statErr)
	}
	if result.StagedPath != target+".new" {
		t.Fatalf("staged path = %s", result.StagedPath)
	}
}

// TestCopyCommandIsPlatformExecutable verifies rollback guidance matches the target shell.
func TestCopyCommandIsPlatformExecutable(t *testing.T) {
	win := copyCommand(`C:\Users\me\wo backup.exe`, `C:\Program Files\wo\wo.exe`, "windows")
	if strings.Contains(win, "cp ") || !strings.Contains(win, "powershell -NoProfile -Command") || !strings.Contains(win, "Copy-Item") {
		t.Fatalf("windows command = %q, want PowerShell Copy-Item without cp", win)
	}
	unix := copyCommand("/tmp/wo backup", "/usr/local/bin/wo", "linux")
	if !strings.HasPrefix(unix, "cp ") || !strings.Contains(unix, "'/tmp/wo backup'") {
		t.Fatalf("unix command = %q, want quoted cp", unix)
	}
}

// TestPrintInstallResultUsesWindowsCommands verifies Windows rollback text is executable.
func TestPrintInstallResultUsesWindowsCommands(t *testing.T) {
	rollback := copyCommand(`C:\backup\wo.exe`, `C:\tools\wo.exe`, "windows")
	result := updateInstallResult{
		Tool:       "wo",
		Current:    "v1.0.0",
		Latest:     "v1.1.0",
		TargetPath: `C:\tools\wo.exe`,
		BackupPath: `C:\backup\wo.exe`,
		Rollback:   rollback,
		Updated:    true,
		Pending:    true,
	}
	var out bytes.Buffer
	printInstallResult(&out, result)
	if strings.Contains(out.String(), "cp ") || !strings.Contains(out.String(), rollback) {
		t.Fatalf("windows pending output = %q, want PowerShell rollback", out.String())
	}

	out.Reset()
	result.Updated = false
	result.Pending = false
	result.Err = fmt.Errorf("replace failed")
	printInstallResult(&out, result)
	if strings.Contains(out.String(), "cp ") || !strings.Contains(out.String(), rollback) {
		t.Fatalf("windows failure output = %q, want PowerShell rollback", out.String())
	}
}

// TestPrintInstallResultReportsMissingOz verifies oz absence is explicit and independent.
func TestPrintInstallResultReportsMissingOz(t *testing.T) {
	var out bytes.Buffer
	printInstallResult(&out, updateInstallResult{Tool: "oz", Err: fmt.Errorf("找不到 oz 可执行文件")})
	if !strings.Contains(out.String(), "oz: 更新失败") || !strings.Contains(out.String(), "找不到 oz") {
		t.Fatalf("output = %q", out.String())
	}
}

func updateServer(t *testing.T, tool, latest string, binary []byte, badChecksum bool) (*httptest.Server, []byte) {
	t.Helper()
	archive := tarGzBinary(t, tool, binary)
	digest := sha256.Sum256(archive)
	sum := fmt.Sprintf("%x", digest)
	if badChecksum {
		sum = strings.Repeat("0", 64)
	}
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assetName := fmt.Sprintf("%s_%s_%s_%s.tar.gz", tool, latest, runtime.GOOS, runtime.GOARCH)
		switch r.URL.Path {
		case "/repos/xbugs221/" + tool + "/releases/latest", "/repos/xbugs221/oz/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":"sha256sums.txt","browser_download_url":%q}]}`, latest, assetName, server.URL+"/"+tool+".tgz", server.URL+"/sha256sums.txt")
		case "/" + tool + ".tgz":
			w.Write(archive)
		case "/sha256sums.txt":
			fmt.Fprintf(w, "%s  %s\n", sum, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, archive
}

func fakeBinary(version string) []byte {
	return []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo " + version + "; else exit 0; fi\n")
}

func tarGzBinary(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
