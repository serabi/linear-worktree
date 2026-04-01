package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmuxFallbackUsesMessageWorktreePath(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	tmuxPath := filepath.Join(tmpDir, "tmux")
	tmuxScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"has-session\" ]; then\n" +
		"  exit 1\n" +
		"fi\n" +
		"if [ \"$1\" = \"new-session\" ]; then\n" +
		"  printf '%s\\n' \"$@\" > \"" + logPath + "\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(tmuxPath, []byte(tmuxScript), 0755); err != nil {
		t.Fatalf("write tmux script: %v", err)
	}

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", origPath)
	}()

	m := NewModel(Config{
		ClaudeCommand: "claude",
		WorktreeBase:  "/wrong/base",
	})
	m.issues = []Issue{
		{Identifier: "TEST-555", Title: "Test issue"},
	}

	wantPath := "/tmp/expected-worktree"
	_, cmd := m.Update(cmuxSlotOpenedMsg{
		identifier: "TEST-555",
		wtPath:     wantPath,
		err:        errors.New("cmux failed"),
	})
	if cmd == nil {
		t.Fatalf("expected fallback launch cmd")
	}

	msg := cmd()
	launched, ok := msg.(claudeLaunchedMsg)
	if !ok {
		t.Fatalf("expected claudeLaunchedMsg, got %T", msg)
	}
	if launched.identifier != "TEST-555" {
		t.Fatalf("identifier = %q, want %q", launched.identifier, "TEST-555")
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	if !strings.Contains(string(logData), wantPath) {
		t.Fatalf("tmux args did not include fallback wtPath %q: %s", wantPath, string(logData))
	}
}
