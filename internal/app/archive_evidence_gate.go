// Package app verifies that promoted acceptance evidence is part of the durable archive commit.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// verifyQualityLoopArchivedEvidenceCommit requires a real, tracked evidence package in the proposal archive commit.
func verifyQualityLoopArchivedEvidenceCommit(repo string, state State) error {
	payloadDir, err := qualityLoopArchivePayloadDir(repo, state.ChangeName)
	if err != nil {
		return err
	}
	payloadRelative, err := filepath.Rel(repo, payloadDir)
	if err != nil {
		return err
	}
	payloadRelative = filepath.ToSlash(payloadRelative)
	if !strings.HasPrefix(payloadRelative, "docs/changes/archive/") {
		return fmt.Errorf("archive 提交门禁找不到归档后的提案目录：%s", payloadRelative)
	}

	packageRelative := filepath.ToSlash(filepath.Join("tests", "evidence", "proposals", state.ChangeName))
	packageDir := filepath.Join(repo, filepath.FromSlash(packageRelative))
	files, err := qualityLoopEvidencePackageFiles(repo, packageDir)
	if err != nil {
		return err
	}
	for _, required := range []string{"README.md", "DELIVERY.md", "manifest.json", "result.json"} {
		if !containsArchiveEvidenceFile(files, packageRelative+"/"+required) {
			return fmt.Errorf("archive 提交级证据包缺少 %s", required)
		}
	}
	for _, path := range files {
		ignored, checkErr := gitPathIgnored(repo, path)
		if checkErr != nil {
			return checkErr
		}
		if ignored {
			return fmt.Errorf("archive 提交级证据命中 git ignore：%s", path)
		}
		if err := gitPathTracked(repo, path); err != nil {
			return fmt.Errorf("archive 提交级证据未进入 git 索引：%s: %w", path, err)
		}
	}
	if err := requireCleanArchiveCommitPaths(repo, packageRelative, payloadRelative); err != nil {
		return err
	}
	if strings.TrimSpace(state.DeliveryBaseHead) == "" {
		return nil
	}
	evidenceCommit, err := latestPathCommit(repo, packageRelative)
	if err != nil {
		return err
	}
	proposalCommit, err := latestPathCommit(repo, payloadRelative)
	if err != nil {
		return err
	}
	if evidenceCommit == "" || proposalCommit == "" || evidenceCommit != proposalCommit {
		return fmt.Errorf(
			"archive 提交级证据与归档提案不在同一提交：evidence=%s proposal=%s",
			evidenceCommit,
			proposalCommit,
		)
	}
	deliveryHead, err := verifyQualityLoopDeliveryCommitRange(repo, state.DeliveryBaseHead)
	if err != nil {
		return err
	}
	if deliveryHead != "" {
		if err := requireCleanQualityLoopDelivery(repo, state.ChangeName); err != nil {
			return err
		}
	}
	if deliveryHead != "" && deliveryHead != proposalCommit {
		return fmt.Errorf(
			"archive 交付 HEAD 不是归档提案与证据提交：head=%s archive=%s",
			deliveryHead,
			proposalCommit,
		)
	}
	for _, path := range files {
		fileCommit, commitErr := latestPathCommit(repo, path)
		if commitErr != nil {
			return commitErr
		}
		if fileCommit != proposalCommit {
			return fmt.Errorf(
				"archive 提交级证据文件未随归档提案提交：path=%s evidence=%s proposal=%s",
				path,
				fileCommit,
				proposalCommit,
			)
		}
	}
	return nil
}

// requireCleanQualityLoopDelivery proves implementation content is included in the sole delivery commit.
func requireCleanQualityLoopDelivery(repo, changeName string) error {
	_, status, err := gitSnapshot(repo)
	if err != nil {
		return err
	}
	var pending []string
	for path := range statusLineByPath(status) {
		if isUnrelatedChangePath(path, changeName) {
			continue
		}
		pending = append(pending, path)
	}
	pending = uniqueSortedPaths(pending)
	if len(pending) > 0 {
		return fmt.Errorf("archive 完整交付提交后仍有未提交实现内容：%s", strings.Join(pending, ", "))
	}
	return nil
}

// verifyQualityLoopDeliveryCommitRange requires one complete delivery commit for newly sealed runs.
func verifyQualityLoopDeliveryCommitRange(repo, deliveryBaseHead string) (string, error) {
	deliveryBaseHead = strings.TrimSpace(deliveryBaseHead)
	if deliveryBaseHead == "" {
		return "", nil
	}
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", err
	}
	ancestor := commandContext(context.Background(), gitPath, "merge-base", "--is-ancestor", deliveryBaseHead, "HEAD")
	ancestor.Dir = repo
	if err := ancestor.Run(); err != nil {
		return "", fmt.Errorf("archive delivery_base_head 不是当前 HEAD 的祖先：%s", deliveryBaseHead)
	}
	countCmd := commandContext(context.Background(), gitPath, "rev-list", "--count", deliveryBaseHead+"..HEAD")
	countCmd.Dir = repo
	countOutput, err := countCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("读取 archive 交付提交范围失败：%w", err)
	}
	if count := strings.TrimSpace(string(countOutput)); count != "1" {
		return "", fmt.Errorf("archive 从 delivery_base_head 到 HEAD 必须恰好一个完整交付提交，实际=%s", count)
	}
	headCmd := commandContext(context.Background(), gitPath, "rev-parse", "--verify", "HEAD^{commit}")
	headCmd.Dir = repo
	headOutput, err := headCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("读取 archive 交付 HEAD 失败：%w", err)
	}
	return strings.TrimSpace(string(headOutput)), nil
}

// qualityLoopEvidencePackageFiles returns regular package files and rejects links or special entries.
func qualityLoopEvidencePackageFiles(repo, packageDir string) ([]string, error) {
	info, err := os.Lstat(packageDir)
	if err != nil {
		return nil, fmt.Errorf("archive 提交级证据包不存在：%w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("archive 提交级证据包必须是真实目录：%s", packageDir)
	}
	var files []string
	err = filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == packageDir {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive 提交级证据不能包含符号链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("archive 提交级证据必须是普通文件：%s", path)
		}
		relative, relErr := filepath.Rel(repo, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("archive 提交级证据包为空：%s", packageDir)
	}
	return files, nil
}

// containsArchiveEvidenceFile reports whether the package contains one required relative file.
func containsArchiveEvidenceFile(files []string, required string) bool {
	for _, path := range files {
		if path == required {
			return true
		}
	}
	return false
}

// gitPathIgnored checks ignore rules even for an already tracked path.
func gitPathIgnored(repo, path string) (bool, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return false, err
	}
	cmd := commandContext(context.Background(), gitPath, "check-ignore", "--no-index", "-q", "--", path)
	cmd.Dir = repo
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git check-ignore %s 失败：%w", path, err)
}

// gitPathTracked verifies that the current index contains the promoted evidence file.
func gitPathTracked(repo, path string) error {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return err
	}
	cmd := commandContext(context.Background(), gitPath, "ls-files", "--error-unmatch", "--", path)
	cmd.Dir = repo
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}

// requireCleanArchiveCommitPaths rejects staged, unstaged, or untracked archive payload changes.
func requireCleanArchiveCommitPaths(repo string, paths ...string) error {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return err
	}
	args := []string{"status", "--porcelain=v1", "--untracked-files=all", "--"}
	args = append(args, paths...)
	cmd := commandContext(context.Background(), gitPath, args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("读取 archive 提交状态失败：%w", err)
	}
	if detail := strings.TrimSpace(string(output)); detail != "" {
		return fmt.Errorf("archive 提交级证据或归档提案尚未提交：%s", detail)
	}
	return nil
}

// latestPathCommit returns the newest commit that contains the named archive path.
func latestPathCommit(repo, path string) (string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", err
	}
	cmd := commandContext(context.Background(), gitPath, "log", "-1", "--format=%H", "--", path)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("读取 archive 路径提交失败：%w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
