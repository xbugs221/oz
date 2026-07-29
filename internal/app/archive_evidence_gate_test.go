// Package app tests the final commit boundary for promoted proposal evidence.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyQualityLoopArchivedEvidenceCommit enforces tracking, ignore, cleanliness, and commit ownership.
func TestVerifyQualityLoopArchivedEvidenceCommit(t *testing.T) {
	tests := []struct {
		name      string
		arrange   func(*workflowFixture, string, string)
		wantError string
	}{
		{
			name: "same archive commit",
			arrange: func(f *workflowFixture, archiveDir, packageDir string) {
				f.git("add", archiveDir, packageDir)
				f.git("commit", "-q", "-m", "archive with evidence")
			},
		},
		{
			name: "untracked evidence",
			arrange: func(f *workflowFixture, archiveDir, _ string) {
				f.git("add", archiveDir)
				f.git("commit", "-q", "-m", "archive without evidence")
			},
			wantError: "未进入 git 索引",
		},
		{
			name: "ignored evidence",
			arrange: func(f *workflowFixture, archiveDir, packageDir string) {
				f.git("add", packageDir)
				f.git("commit", "-q", "-m", "seed tracked evidence")
				if err := os.WriteFile(filepath.Join(f.repo, ".gitignore"), []byte("tests/evidence/\n"), 0o644); err != nil {
					f.t.Fatal(err)
				}
				f.git("add", archiveDir, ".gitignore")
				f.git("commit", "-q", "-m", "archive ignored evidence")
			},
			wantError: "命中 git ignore",
		},
		{
			name: "separate evidence commit",
			arrange: func(f *workflowFixture, archiveDir, packageDir string) {
				f.git("add", archiveDir)
				f.git("commit", "-q", "-m", "archive proposal")
				f.git("add", packageDir)
				f.git("commit", "-q", "-m", "late evidence")
			},
			wantError: "不在同一提交",
		},
		{
			name: "dirty evidence",
			arrange: func(f *workflowFixture, archiveDir, packageDir string) {
				f.git("add", archiveDir, packageDir)
				f.git("commit", "-q", "-m", "archive with evidence")
				if err := os.WriteFile(filepath.Join(f.repo, packageDir, "result.json"), []byte("{\"dirty\":true}\n"), 0o644); err != nil {
					f.t.Fatal(err)
				}
			},
			wantError: "尚未提交",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			fixture := &workflowFixture{t: t, repo: repo}
			changeName := "1-demo"
			archiveRelative := filepath.Join("docs", "changes", "archive", "2026-07-29-"+changeName)
			packageRelative := filepath.Join("tests", "evidence", "proposals", changeName)
			writeArchiveEvidenceFixture(t, repo, archiveRelative, packageRelative)
			baseHead, err := latestPathCommit(repo, "README.md")
			if err != nil {
				t.Fatal(err)
			}
			tc.arrange(fixture, archiveRelative, packageRelative)

			err = verifyQualityLoopArchivedEvidenceCommit(repo, State{
				ChangeName:       changeName,
				DeliveryBaseHead: baseHead,
			})
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("valid archive evidence commit rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("archive evidence error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

// TestVerifyQualityLoopDeliveryCommitRange requires one post-base commit while preserving legacy runs.
func TestVerifyQualityLoopDeliveryCommitRange(t *testing.T) {
	for _, extraCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "one delivery commit", true: "two delivery commits"}[extraCommit], func(t *testing.T) {
			repo := gitRepo(t)
			fixture := &workflowFixture{t: t, repo: repo}
			baseHead, err := latestPathCommit(repo, "README.md")
			if err != nil {
				t.Fatal(err)
			}
			if extraCommit {
				if err := os.WriteFile(filepath.Join(repo, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				fixture.git("add", "implementation.go")
				fixture.git("commit", "-q", "-m", "implementation outside delivery")
			}
			changeName := "1-demo"
			archiveRelative := filepath.Join("docs", "changes", "archive", "2026-07-29-"+changeName)
			packageRelative := filepath.Join("tests", "evidence", "proposals", changeName)
			writeArchiveEvidenceFixture(t, repo, archiveRelative, packageRelative)
			fixture.git("add", archiveRelative, packageRelative)
			fixture.git("commit", "-q", "-m", "archive complete delivery")

			err = verifyQualityLoopArchivedEvidenceCommit(repo, State{
				ChangeName:       changeName,
				DeliveryBaseHead: baseHead,
			})
			if !extraCommit && err != nil {
				t.Fatalf("single delivery commit rejected: %v", err)
			}
			if extraCommit && (err == nil || !strings.Contains(err.Error(), "恰好一个")) {
				t.Fatalf("multiple delivery commits error = %v", err)
			}
		})
	}
}

// TestVerifyQualityLoopDeliveryCommitRejectsDirtyImplementation requires source bytes to belong to HEAD.
func TestVerifyQualityLoopDeliveryCommitRejectsDirtyImplementation(t *testing.T) {
	repo := gitRepo(t)
	fixture := &workflowFixture{t: t, repo: repo}
	baseHead, err := latestPathCommit(repo, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	changeName := "1-demo"
	archiveRelative := filepath.Join("docs", "changes", "archive", "2026-07-29-"+changeName)
	packageRelative := filepath.Join("tests", "evidence", "proposals", changeName)
	writeArchiveEvidenceFixture(t, repo, archiveRelative, packageRelative)
	fixture.git("add", archiveRelative, packageRelative)
	fixture.git("commit", "-q", "-m", "archive complete delivery")
	if err := os.WriteFile(filepath.Join(repo, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = verifyQualityLoopArchivedEvidenceCommit(repo, State{
		ChangeName:       changeName,
		DeliveryBaseHead: baseHead,
	})
	if err == nil || !strings.Contains(err.Error(), "未提交实现内容") {
		t.Fatalf("dirty implementation error = %v", err)
	}
}

// TestCreateRunSealsDeliveryBaseHead binds the eventual delivery commit to the repository at run start.
func TestCreateRunSealsDeliveryBaseHead(t *testing.T) {
	repo, changeName, _, head, _ := newRepairEvidenceFixture(t)
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: &fakeWorkflowRunner{}})
	state, err := NewEngine(repo, registry).createRun(changeName)
	if err != nil {
		t.Fatal(err)
	}
	if state.DeliveryBaseHead != head || state.DeliveryBaseHead == "" {
		t.Fatalf("delivery_base_head = %q, want %q", state.DeliveryBaseHead, head)
	}
}

// writeArchiveEvidenceFixture creates one archived proposal and the minimum promoted evidence package.
func writeArchiveEvidenceFixture(t *testing.T, repo, archiveRelative, packageRelative string) {
	t.Helper()
	files := map[string]string{
		filepath.Join(archiveRelative, "brief.md"):  "# archived proposal\n",
		filepath.Join(packageRelative, "README.md"): "# evidence\n",
		filepath.Join(packageRelative, "DELIVERY.md"): `# 交付报告

审核人员可按用户路径完成操作，并直接查看最终结果。
`,
		filepath.Join(packageRelative, "manifest.json"): `{"version":1}
`,
		filepath.Join(packageRelative, "result.json"): `{"status":"passed"}
`,
		filepath.Join(packageRelative, "demo.log"): "demo passed\n",
	}
	for relative, body := range files {
		path := filepath.Join(repo, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
