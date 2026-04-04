package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Path   string
	Branch string
	Head   string
	Bare   bool
}

func FindRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func ListWorktrees() ([]Worktree, error) {
	root, err := FindRepoRoot()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: line[9:]}
		case strings.HasPrefix(line, "HEAD "):
			current.Head = line[5:]
		case strings.HasPrefix(line, "branch "):
			branchRef := line[7:]
			current.Branch = strings.TrimPrefix(branchRef, "refs/heads/")
		case line == "bare":
			current.Bare = true
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

func CreateWorktree(identifier string, cfg Config) (string, error) {
	root, err := FindRepoRoot()
	if err != nil {
		return "", err
	}

	branchName := cfg.BranchPrefix + strings.ToLower(identifier)
	worktreeBase := cfg.WorktreeBase
	if !filepath.IsAbs(worktreeBase) {
		worktreeBase = filepath.Join(root, worktreeBase)
	}

	wtPath := filepath.Join(worktreeBase, strings.ToLower(identifier))

	// Prevent path traversal
	cleanBase := filepath.Clean(worktreeBase)
	cleanPath := filepath.Clean(wtPath)
	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid identifier: path escapes worktree base")
	}

	// Already exists?
	if _, err := os.Stat(wtPath); err == nil {
		return wtPath, nil
	}

	if err := os.MkdirAll(worktreeBase, 0700); err != nil {
		return "", fmt.Errorf("creating worktree base: %w", err)
	}

	// Check if branch exists
	checkCmd := exec.Command("git", "branch", "--list", branchName)
	checkCmd.Dir = root
	checkOut, _ := checkCmd.Output()

	var cmd *exec.Cmd
	if strings.TrimSpace(string(checkOut)) != "" {
		cmd = exec.Command("git", "worktree", "add", wtPath, branchName)
	} else {
		cmd = exec.Command("git", "worktree", "add", "-b", branchName, wtPath)
	}
	cmd.Dir = root

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add failed: %s", string(out))
	}

	// Copy config files
	for _, f := range cfg.CopyFiles {
		src := filepath.Join(root, f)
		dst := filepath.Join(wtPath, f)
		_ = copyFile(src, dst)
	}
	for _, d := range cfg.CopyDirs {
		src := filepath.Join(root, d)
		dst := filepath.Join(wtPath, d)
		_ = copyDir(src, dst)
	}

	return wtPath, nil
}

// BuildRemoveWorktreeMessage returns a confirm-dialog message for removing a
// worktree. If wtPath is a dirty worktree, a warning line is prepended. An
// empty wtPath skips the dirty check (used when the path is unknown).
func BuildRemoveWorktreeMessage(wtPath, label string) string {
	base := fmt.Sprintf("Remove worktree and branch %s? This cannot be undone.\n(The cmux slot will also be closed if open.)", label)
	if wtPath == "" {
		return base
	}
	dirty, summary, err := WorktreeDirty(wtPath)
	if err != nil || !dirty {
		return base
	}
	return fmt.Sprintf("WARNING: %s. This work will be lost.\n\n%s", summary, base)
}

// WorktreeDirty reports whether the worktree at wtPath has uncommitted changes,
// untracked files, or unpushed commits. The summary string is a short,
// human-readable description suitable for inclusion in a confirm dialog
// (e.g. "3 uncommitted, 2 unpushed"). An empty summary means not dirty.
func WorktreeDirty(wtPath string) (bool, string, error) {
	if _, err := os.Stat(wtPath); err != nil {
		return false, "", err
	}

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = wtPath
	statusOut, err := statusCmd.Output()
	if err != nil {
		return false, "", fmt.Errorf("git status: %w", err)
	}

	uncommitted := 0
	for _, line := range strings.Split(strings.TrimRight(string(statusOut), "\n"), "\n") {
		if line != "" {
			uncommitted++
		}
	}

	// Best-effort unpushed check. A missing upstream is not an error: treat as
	// zero unpushed for dialog purposes (the user will still see uncommitted
	// counts and the branch will be deleted).
	unpushed := 0
	logCmd := exec.Command("git", "log", "@{u}..", "--oneline")
	logCmd.Dir = wtPath
	if logOut, logErr := logCmd.Output(); logErr == nil {
		for _, line := range strings.Split(strings.TrimRight(string(logOut), "\n"), "\n") {
			if line != "" {
				unpushed++
			}
		}
	}

	if uncommitted == 0 && unpushed == 0 {
		return false, "", nil
	}

	parts := []string{}
	if uncommitted > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted", uncommitted))
	}
	if unpushed > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed", unpushed))
	}
	return true, strings.Join(parts, ", "), nil
}

func RemoveWorktree(wtPath string) error {
	root, err := FindRepoRoot()
	if err != nil {
		return err
	}

	// Find branch name
	worktrees, _ := ListWorktrees()
	var branch string
	targetPath := normalizePath(wtPath)
	for _, wt := range worktrees {
		if normalizePath(wt.Path) == targetPath {
			branch = wt.Branch
			break
		}
	}

	cmd := exec.Command("git", "worktree", "remove", wtPath, "--force")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove failed: %s", string(out))
	}

	if branch != "" {
		cmd = exec.Command("git", "branch", "-D", branch)
		cmd.Dir = root
		// Best-effort branch cleanup: the branch may have already been deleted
		// or may be checked out elsewhere. The worktree itself has already been
		// removed, so failing here is acceptable.
		_ = cmd.Run()
	}

	return nil
}

func normalizePath(p string) string {
	cleaned := filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		if closeErr != nil {
			return fmt.Errorf("copying file: %w (also failed to close destination: %v)", copyErr, closeErr)
		}
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}
