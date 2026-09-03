// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
)

const gitSHALength = 40

type GitSource struct{}

func (*GitSource) Handles(loc Locator) bool {
	return IsGitSourceURL(loc.URL) || (loc.Ref != "" && isLocalGitWorktree(loc.URL))
}

func (*GitSource) Fetch(ctx context.Context, loc Locator, dstPath string) error {
	repoURL, ref := loc.URL, loc.Ref

	auth, err := gitAuth(loc.URL)
	if err != nil {
		return err
	}

	refName, commitHash, err := getRefName(ctx, repoURL, ref, auth)
	if err != nil {
		return err
	}

	// refName is empty when ref is a commit SHA not pointed to by any named
	// ref; a full-depth clone is required to make the commit reachable.
	repo, err := git.PlainOpen(dstPath)
	if err != nil {
		return cloneRepo(ctx, repoURL, dstPath, refName, commitHash, auth)
	}

	fetchOpts := &git.FetchOptions{
		Force: true,
		Prune: true,
		Auth:  auth,
	}
	if refName != "" {
		fetchOpts.Depth = 1
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil &&
		!errors.Is(err, git.NoErrAlreadyUpToDate) {
		return err
	}

	if refName == "" {
		worktree, err := repo.Worktree()
		if err != nil {
			return err
		}

		return worktree.Reset(&git.ResetOptions{Commit: commitHash, Mode: git.HardReset})
	}

	head, err := repo.Head()
	if err != nil {
		return err
	}

	if !head.Name().IsBranch() {
		// Detached HEAD (tag checkout) — tags are immutable, nothing to update.
		return nil
	}

	remoteRef, err := repo.Reference(
		plumbing.NewRemoteReferenceName("origin", head.Name().Short()),
		true,
	)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil
		}

		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	return worktree.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	})
}

func cloneRepo(
	ctx context.Context,
	repoURL, dstPath string,
	refName plumbing.ReferenceName,
	commitHash plumbing.Hash,
	auth transport.AuthMethod,
) error {
	opts := &git.CloneOptions{
		URL:  repoURL,
		Auth: auth,
	}
	if refName != "" {
		opts.ReferenceName = refName
		opts.SingleBranch = true
		opts.Depth = 1
	}

	cloned, err := git.PlainCloneContext(ctx, dstPath, false, opts)
	if err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	if refName == "" {
		worktree, err := cloned.Worktree()
		if err != nil {
			return err
		}

		return worktree.Reset(&git.ResetOptions{Commit: commitHash, Mode: git.HardReset})
	}

	return nil
}

func IsGitSourceURL(url string) bool {
	return strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "git://") ||
		((strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")) &&
			strings.HasSuffix(url, ".git"))
}

func isLocalGitWorktree(url string) bool {
	if strings.Contains(url, "://") || strings.HasPrefix(url, "file:") {
		return false
	}

	_, err := os.Stat(filepath.Join(url, ".git"))

	return err == nil
}

// getRefName resolves ref against the remote and returns the canonical
// reference name and the target commit hash. When refName is empty, no named
// ref points to the commit and a full-depth clone is required.
func getRefName(
	ctx context.Context,
	repoURL, ref string,
	auth transport.AuthMethod,
) (plumbing.ReferenceName, plumbing.Hash, error) {
	remote := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})

	remoteRefs, err := remote.ListContext(ctx, &git.ListOptions{Auth: auth})
	if err != nil {
		return "", plumbing.ZeroHash, fmt.Errorf("listing refs for %s: %w", repoURL, err)
	}

	if ref == "" {
		return resolveHeadRef(remoteRefs, repoURL)
	}

	if isFullCommitSHA(ref) {
		return resolveCommitSHARef(remoteRefs, ref)
	}

	return resolveNamedRef(remoteRefs, ref, repoURL)
}

// resolveHeadRef resolves remote HEAD to the actual default branch so the
// clone produces a proper branch checkout with remote-tracking refs.
func resolveHeadRef(
	remoteRefs []*plumbing.Reference,
	repoURL string,
) (plumbing.ReferenceName, plumbing.Hash, error) {
	for _, r := range remoteRefs {
		if r.Name() == plumbing.HEAD && r.Type() == plumbing.SymbolicReference {
			target := r.Target()
			for _, br := range remoteRefs {
				if br.Name() == target {
					return target, br.Hash(), nil
				}
			}

			return "", plumbing.ZeroHash, fmt.Errorf(
				"remote HEAD points to %q which does not exist in %s",
				target,
				repoURL,
			)
		}
	}

	return "", plumbing.ZeroHash, fmt.Errorf("no HEAD reference found for %s", repoURL)
}

func resolveCommitSHARef(
	remoteRefs []*plumbing.Reference,
	ref string,
) (plumbing.ReferenceName, plumbing.Hash, error) {
	targetHash := plumbing.NewHash(ref)
	for _, r := range remoteRefs {
		if r.Type() == plumbing.SymbolicReference {
			continue
		}
		if r.Hash() == targetHash {
			return r.Name(), targetHash, nil
		}
	}
	// No named ref points to this SHA; full-depth clone required.
	return "", targetHash, nil
}

func resolveNamedRef(
	remoteRefs []*plumbing.Reference,
	ref, repoURL string,
) (plumbing.ReferenceName, plumbing.Hash, error) {
	for _, r := range remoteRefs {
		if r.Type() == plumbing.SymbolicReference {
			continue
		}
		if r.Name().Short() == ref || r.Name().String() == ref {
			return r.Name(), r.Hash(), nil
		}
	}

	return "", plumbing.ZeroHash, fmt.Errorf("ref %q not found in %s", ref, repoURL)
}

// Probe: named refs require a remote lookup because they can move; full SHAs do not.
func (*GitSource) Probe(ctx context.Context, loc Locator) (Probe, error) {
	if isFullCommitSHA(loc.Ref) {
		return Probe{Identity: gitIdentity(loc.Ref)}, nil
	}
	auth, err := gitAuth(loc.URL)
	if err != nil {
		return Probe{}, err
	}
	_, hash, err := getRefName(ctx, loc.URL, loc.Ref, auth)
	if err != nil {
		return Probe{}, err
	}

	return Probe{Identity: gitIdentity(hash.String())}, nil
}

func gitIdentity(commit string) string {
	return "git-commit:" + commit
}

func isFullCommitSHA(s string) bool {
	if len(s) != gitSHALength {
		return false
	}
	for _, c := range strings.ToLower(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

func gitAuth(url string) (transport.AuthMethod, error) {
	if !strings.HasPrefix(url, "git@") {
		return nil, nil //nolint:nilnil
	}

	if auth, err := gogitssh.NewSSHAgentAuth("git"); err == nil {
		return auth, nil
	}

	return nil, fmt.Errorf(
		"no SSH credentials available for %s (SSH agent not running)",
		url,
	)
}
