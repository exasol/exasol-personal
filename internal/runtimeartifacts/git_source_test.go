// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestIsGitSourceURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url  string
		want bool
	}{
		{"git@github.com:org/repo.git", true},
		{"git@github.com:org/repo", true},
		{"git://github.com/org/repo.git", true},
		{"https://github.com/org/repo.git", true},
		{"http://github.com/org/repo.git", true},
		{"https://github.com/org/repo", false},
		{"http://example.com/archive.tar.gz", false},
		{"file:///path/to/dir", false},
		{"", false},
	}

	for _, tc := range cases {
		got := IsGitSourceURL(tc.url)
		if got != tc.want {
			t.Errorf("IsGitSourceURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestParseGitURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url     string
		wantURL string
		wantRef string
	}{
		{"https://github.com/org/repo.git", "https://github.com/org/repo.git", ""},
		{"https://github.com/org/repo.git@main", "https://github.com/org/repo.git", "main"},
		{
			"https://github.com/org/repo.git@a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			"https://github.com/org/repo.git",
			"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{"http://github.com/org/repo.git", "http://github.com/org/repo.git", ""},
		{"git://github.com/org/repo.git", "git://github.com/org/repo.git", ""},
		{"git@github.com:org/repo.git", "git@github.com:org/repo.git", ""},
		{"git@github.com:org/repo.git@main", "git@github.com:org/repo.git", "main"},
		{"https://example.com/preset.tar.gz", "https://example.com/preset.tar.gz", ""},
		{"file:///some/path/to/preset", "file:///some/path/to/preset", ""},
	}

	for _, tc := range cases {
		gotURL, gotRef := ParseGitURL(tc.url)
		if gotURL != tc.wantURL || gotRef != tc.wantRef {
			t.Errorf("ParseGitURL(%q) = (%q, %q), want (%q, %q)",
				tc.url, gotURL, gotRef, tc.wantURL, tc.wantRef)
		}
	}
}

func TestGitSource_CanFetch_RemoteURLs(t *testing.T) {
	t.Parallel()

	src := &GitSource{}
	trueURLs := []string{
		"git@github.com:org/repo.git",
		"git@github.com:org/repo",
		"https://github.com/org/repo.git",
		"http://github.com/org/repo.git",
		"git://github.com/org/repo.git",
	}
	for _, url := range trueURLs {
		if !src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}

	falseURLs := []string{
		"https://example.com/archive.tar.gz",
		"http://example.com/archive.tar.gz",
		"file:///path/to/dir",
		"",
	}
	for _, url := range falseURLs {
		if src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}

func TestGitAuth_NilForNonSSHURLs(t *testing.T) {
	t.Parallel()

	for _, rawURL := range []string{
		"https://github.com/org/repo.git",
		"http://github.com/org/repo.git",
		"git://github.com/org/repo.git",
		"file:///path/to/repo",
	} {
		auth, err := gitAuth(rawURL)
		if err != nil {
			t.Errorf("gitAuth(%q) unexpected error: %v", rawURL, err)
		}
		if auth != nil {
			t.Errorf("gitAuth(%q) expected nil auth, got %T", rawURL, auth)
		}
	}
}

func TestGitSource_Fetch_ClonesLocalRepo(t *testing.T) {
	t.Parallel()

	repoDir, headHash := createTestGitRepo(t, map[string]string{
		"README.md": "hello world",
	})
	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir, cloneDir); err != nil {
		t.Fatalf("expected clone to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "README.md")); err != nil {
		t.Fatalf("expected README.md in clone, got %v", err)
	}

	repo, err := git.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("expected valid git repo at clone dir, got %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("expected HEAD reference, got %v", err)
	}
	if head.Hash().String() != headHash {
		t.Fatalf("expected HEAD %s, got %s", headHash, head.Hash().String())
	}
}

func TestGitSource_Fetch_UpdatesWorkingTree(t *testing.T) {
	t.Parallel()

	repoDir, _ := createTestGitRepo(t, map[string]string{"file.txt": "v1"})
	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir, cloneDir); err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	addCommitToTestRepo(t, repoDir, "file.txt", "v2")

	if _, err := src.Fetch(context.Background(), repoDir, cloneDir); err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	if err != nil {
		t.Fatalf("expected file after update, got %v", err)
	}
	if string(content) != "v2" {
		t.Fatalf("expected v2 content after working tree update, got %q", string(content))
	}
}

func TestGitSource_Fetch_BranchRef(t *testing.T) {
	t.Parallel()

	repoDir, _ := createTestGitRepo(t, map[string]string{"main.txt": "on main"})
	addBranchToTestRepo(t, repoDir, "feature", "feature.txt", "on feature")
	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir+"@feature", cloneDir); err != nil {
		t.Fatalf("expected branch clone to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "feature.txt")); err != nil {
		t.Fatalf("expected feature.txt from branch, got %v", err)
	}
}

func TestGitSource_Fetch_TagRef(t *testing.T) {
	t.Parallel()

	repoDir, _ := createTestGitRepo(t, map[string]string{"release.txt": "v1.0.0"})
	tagHash := addTagToTestRepo(t, repoDir, "v1.0.0")
	addCommitToTestRepo(t, repoDir, "after-tag.txt", "after tag")
	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir+"@v1.0.0", cloneDir); err != nil {
		t.Fatalf("expected tag clone to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "release.txt")); err != nil {
		t.Fatalf("expected release.txt from tag, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "after-tag.txt")); err == nil {
		t.Fatal("expected after-tag.txt to not be present at tag checkout")
	}

	repo, err := git.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("expected valid repo: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("expected HEAD: %v", err)
	}
	if head.Hash().String() != tagHash {
		t.Fatalf("expected tag commit %s, got %s", tagHash, head.Hash().String())
	}
}

func TestGitSource_Fetch_IdempotentOnSameCommit(t *testing.T) {
	t.Parallel()

	repoDir, _ := createTestGitRepo(t, map[string]string{"file.txt": "content"})
	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir, cloneDir); err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	// Corrupt a file; second fetch (no new commit) should not change it.
	corruptedPath := filepath.Join(cloneDir, "file.txt")
	if err := os.WriteFile(corruptedPath, []byte("corrupted"), filePerm); err != nil {
		t.Fatalf("corrupt failed: %v", err)
	}
	if _, err := src.Fetch(context.Background(), repoDir, cloneDir); err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	// Reset resets to the remote ref, which has the same commit — no real
	// change is introduced, so the hard reset replaces the corrupted content.
	if string(content) != "content" {
		t.Fatalf("expected hard reset to restore original content, got %q", string(content))
	}
}

func TestGitSource_Identify_NoRef(t *testing.T) {
	t.Parallel()

	repoDir, headHash := createTestGitRepo(t, map[string]string{"file.txt": "content"})
	src := &GitSource{}

	hash, err := src.Identify(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != headHash {
		t.Fatalf("expected %s, got %s", headHash, hash)
	}
}

func TestGitSource_Identify_BranchRef(t *testing.T) {
	t.Parallel()

	repoDir, _ := createTestGitRepo(t, map[string]string{"main.txt": "main"})
	addBranchToTestRepo(t, repoDir, "feature", "feature.txt", "feature")

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	featureRef, err := repo.Reference(plumbing.NewBranchReferenceName("feature"), true)
	if err != nil {
		t.Fatalf("get feature ref: %v", err)
	}
	wantHash := featureRef.Hash().String()

	src := &GitSource{}
	hash, err := src.Identify(context.Background(), repoDir+"@feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("expected %s, got %s", wantHash, hash)
	}
}

func TestGitSource_Identify_CommitSHA(t *testing.T) {
	t.Parallel()

	repoDir, headHash := createTestGitRepo(t, map[string]string{"file.txt": "content"})
	src := &GitSource{}

	// Full SHA embedded in URL — must return immediately without any network/local call.
	hash, err := src.Identify(context.Background(), repoDir+"@"+headHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != headHash {
		t.Fatalf("expected %s, got %s", headHash, hash)
	}
}

func TestGitSource_Fetch_CommitSHA(t *testing.T) {
	t.Parallel()

	repoDir, headHash := createTestGitRepo(t, map[string]string{
		"pinned.txt": "pinned content",
	})
	addCommitToTestRepo(t, repoDir, "newer.txt", "newer content")

	dstDir := t.TempDir()
	cloneDir := filepath.Join(dstDir, "clone")
	src := &GitSource{}

	if _, err := src.Fetch(context.Background(), repoDir+"@"+headHash, cloneDir); err != nil {
		t.Fatalf("expected SHA clone to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "pinned.txt")); err != nil {
		t.Fatalf("expected pinned.txt in clone, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "newer.txt")); err == nil {
		t.Fatal("expected newer.txt to not be present at pinned commit")
	}

	repo, err := git.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("expected valid repo: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("expected HEAD: %v", err)
	}
	if head.Hash().String() != headHash {
		t.Fatalf("expected HEAD %s, got %s", headHash, head.Hash().String())
	}
}

//nolint:nonamedreturns
func createTestGitRepo(t *testing.T, files map[string]string) (repoPath, headHash string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), dirPerm); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), filePerm); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := worktree.Add(name); err != nil {
			t.Fatalf("git add %s: %v", name, err)
		}
	}

	hash, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Unix(1, 0)},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	return dir, hash.String()
}

func addCommitToTestRepo(t *testing.T, repoPath, fileName, content string) {
	t.Helper()

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	full := filepath.Join(repoPath, fileName)
	if err := os.WriteFile(full, []byte(content), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := worktree.Add(fileName); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := worktree.Commit("add "+fileName, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Unix(2, 0)},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func addBranchToTestRepo(t *testing.T, repoPath, branchName, fileName, content string) {
	t.Helper()

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	if err := worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	}); err != nil {
		t.Fatalf("checkout branch: %v", err)
	}

	full := filepath.Join(repoPath, fileName)
	if err := os.WriteFile(full, []byte(content), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := worktree.Add(fileName); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := worktree.Commit("add "+fileName, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Unix(3, 0)},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	}); err != nil {
		if err := worktree.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName("main"),
		}); err != nil {
			t.Fatalf("checkout back to default: %v", err)
		}
	}
}

func addTagToTestRepo(t *testing.T, repoPath, tagName string) string {
	t.Helper()

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	tagRef := plumbing.NewTagReferenceName(tagName)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(tagRef, head.Hash())); err != nil {
		t.Fatalf("set tag: %v", err)
	}

	return head.Hash().String()
}
