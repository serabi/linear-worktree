package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %s: %v", args, string(out), err)
		}
	}

	return dir
}

func TestCreateWorktree(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")

	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
		CopyFiles:    []string{},
		CopyDirs:     []string{},
	}

	// Create a test file to copy
	if err := os.WriteFile(filepath.Join(repoDir, ".env"), []byte("SECRET=123"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	cfg.CopyFiles = []string{".env"}

	// Change to repo dir for FindRepoRoot
	origDir, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("getwd: %v", wdErr)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("TEST-123", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() error: %v", err)
	}

	// Verify worktree was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("worktree path %s does not exist", path)
	}

	// Verify .env was copied
	envPath := filepath.Join(path, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Error(".env was not copied to worktree")
	}

	// Verify branch was created
	cmd := exec.Command("git", "branch", "--list", "feature/test-123")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch list: %v", err)
	}
	if len(out) == 0 {
		t.Error("branch feature/test-123 was not created")
	}

	// Creating again should return the same path (idempotent)
	path2, err := CreateWorktree("TEST-123", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() second call error: %v", err)
	}
	if path != path2 {
		t.Errorf("second call returned different path: %s vs %s", path, path2)
	}
}

func TestCreateWorktreeRelativeBase(t *testing.T) {
	repoDir := setupTestRepo(t)
	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: ".worktrees",
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("TEST-124", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() error: %v", err)
	}

	wantPath := filepath.Join(repoDir, ".worktrees", "test-124")
	if normalizePath(path) != normalizePath(wantPath) {
		t.Fatalf("CreateWorktree() path = %q, want %q", path, wantPath)
	}
}

func TestListWorktrees(t *testing.T) {
	repoDir := setupTestRepo(t)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	wts, err := ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees() error: %v", err)
	}

	// Should have at least the main worktree
	if len(wts) < 1 {
		t.Fatal("expected at least 1 worktree (main)")
	}
}

func TestListWorktreesPreservesBranchNameWithSlash(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("TEST-123", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() error: %v", err)
	}

	wts, err := ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees() error: %v", err)
	}

	for _, wt := range wts {
		if wt.Branch == "feature/test-123" {
			return
		}
	}
	t.Fatalf("created worktree with branch %q not found in list (path: %q)", "feature/test-123", path)
}

func TestRemoveWorktree(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")

	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("TEST-999", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() error: %v", err)
	}

	err = RemoveWorktree(path)
	if err != nil {
		t.Fatalf("RemoveWorktree() error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("worktree directory should not exist after removal")
	}
}

func TestRemoveWorktreeDeletesSlashBranch(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("TEST-777", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree() error: %v", err)
	}

	if err := RemoveWorktree(path); err != nil {
		t.Fatalf("RemoveWorktree() error: %v", err)
	}

	cmd := exec.Command("git", "branch", "--list", "feature/test-777")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch list: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("branch feature/test-777 still exists after remove: %q", string(out))
	}
}

func TestWorktreeDirty(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	path, err := CreateWorktree("DIRTY-1", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Clean worktree should report not dirty.
	dirty, summary, err := WorktreeDirty(path)
	if err != nil {
		t.Fatalf("WorktreeDirty clean: %v", err)
	}
	if dirty {
		t.Errorf("clean worktree reported dirty: %q", summary)
	}

	// Add an untracked file.
	if err := os.WriteFile(filepath.Join(path, "new.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dirty, summary, err = WorktreeDirty(path)
	if err != nil {
		t.Fatalf("WorktreeDirty untracked: %v", err)
	}
	if !dirty {
		t.Errorf("worktree with untracked file not reported dirty")
	}
	if !strings.Contains(summary, "uncommitted") {
		t.Errorf("summary %q should mention uncommitted", summary)
	}
}

func TestBuildRemoveWorktreeMessage(t *testing.T) {
	// Empty path skips dirty check.
	msg := BuildRemoveWorktreeMessage("", "feature/foo")
	if !strings.Contains(msg, "feature/foo") {
		t.Errorf("message missing label: %q", msg)
	}
	if strings.Contains(msg, "WARNING") {
		t.Errorf("empty path should not produce warning: %q", msg)
	}
}

func TestBuildRemoveWorktreeMessageDirty(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	path, err := CreateWorktree("DIRTY-MSG", cfg)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Make it dirty with an untracked file.
	if err := os.WriteFile(filepath.Join(path, "new.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	msg := BuildRemoveWorktreeMessage(path, "feature/dirty-msg")
	if !strings.Contains(msg, "WARNING") {
		t.Errorf("dirty worktree should produce WARNING: %q", msg)
	}
	if !strings.Contains(msg, "uncommitted") {
		t.Errorf("warning should mention uncommitted: %q", msg)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	if err := os.WriteFile(src, []byte("hello world"), 0755); err != nil {
		t.Fatalf("write source: %v", err)
	}

	err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("copied content = %q, want 'hello world'", string(data))
	}

	// Verify permissions are preserved
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("copied file mode = %v, want %v", dstInfo.Mode(), srcInfo.Mode())
	}
}

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("one"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "file2.txt"), []byte("two"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	err := copyDir(srcDir, dstDir)
	if err != nil {
		t.Fatalf("copyDir() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	if err != nil || string(data) != "one" {
		t.Errorf("file1.txt not copied correctly")
	}

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt"))
	if err != nil || string(data) != "two" {
		t.Errorf("sub/file2.txt not copied correctly")
	}
}

func TestCreateWorktreePathTraversal(t *testing.T) {
	repoDir := setupTestRepo(t)
	worktreeBase := filepath.Join(t.TempDir(), "worktrees")

	cfg := Config{
		BranchPrefix: "feature/",
		WorktreeBase: worktreeBase,
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore dir: %v", err)
		}
	}()

	traversalInputs := []string{"../../etc", "../../../tmp/evil", "foo/../../bar"}
	for _, id := range traversalInputs {
		_, err := CreateWorktree(id, cfg)
		if err == nil {
			t.Errorf("CreateWorktree(%q) should fail with path traversal error", id)
		}
	}
}

func TestCopyDirNonExistent(t *testing.T) {
	err := copyDir("/nonexistent/path", "/tmp/dest")
	if err != nil {
		t.Errorf("copyDir with nonexistent source should return nil, got: %v", err)
	}
}
