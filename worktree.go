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
			parts := strings.Split(line[7:], "/")
			current.Branch = parts[len(parts)-1]
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
		worktreeBase = filepath.Join(root, "..", "worktrees")
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

	os.MkdirAll(worktreeBase, 0700)

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
		copyFile(src, dst)
	}
	for _, d := range cfg.CopyDirs {
		src := filepath.Join(root, d)
		dst := filepath.Join(wtPath, d)
		copyDir(src, dst)
	}

	return wtPath, nil
}

func RemoveWorktree(wtPath string) error {
	root, err := FindRepoRoot()
	if err != nil {
		return err
	}

	// Find branch name
	worktrees, _ := ListWorktrees()
	var branch string
	for _, wt := range worktrees {
		if wt.Path == wtPath {
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
	defer func() { _ = out.Close() }()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
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
