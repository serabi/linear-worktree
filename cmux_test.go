package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeCmux installs a shell script named "cmux" into a temp dir and
// prepends that dir to PATH for the duration of the test. The script
// prints scriptBody to stdout and exits with exitCode.
func withFakeCmux(t *testing.T, scriptBody string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n"
	if scriptBody != "" {
		script += "cat <<'EOF'\n" + scriptBody + "\nEOF\n"
	}
	if exitCode != 0 {
		script += fmt.Sprintf("exit %d\n", exitCode)
	}
	path := filepath.Join(dir, "cmux")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake cmux: %v", err)
	}
	// Prepend the fake dir to the real PATH so /bin/sh and other utilities
	// remain discoverable for the shebang and cat heredoc.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAgentStatusString(t *testing.T) {
	tests := []struct {
		status   AgentStatus
		icon     string
		label    string
	}{
		{AgentRunning, "●", "running"},
		{AgentIdle, "○", "idle"},
		{AgentWaiting, "◐", "waiting"},
		{AgentInactive, "·", "inactive"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.icon {
			t.Errorf("AgentStatus(%d).String() = %q, want %q", tt.status, got, tt.icon)
		}
		if got := tt.status.Label(); got != tt.label {
			t.Errorf("AgentStatus(%d).Label() = %q, want %q", tt.status, got, tt.label)
		}
	}
}

func TestInferStatus(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected AgentStatus
	}{
		{
			name:     "waiting for permission",
			text:     "Allow this action? [y/n]",
			expected: AgentWaiting,
		},
		{
			name:     "waiting Y/n",
			text:     "Continue? [Y/n]",
			expected: AgentWaiting,
		},
		{
			name:     "running with interrupt hint",
			text:     "Reading files... press ctrl+c to interrupt",
			expected: AgentRunning,
		},
		{
			name:     "idle at prompt",
			text:     "─────\n❯ ",
			expected: AgentIdle,
		},
		{
			name:     "idle at shell prompt with newline",
			text:     "done\n> ",
			expected: AgentIdle,
		},
		{
			name:     "idle prompt bare >",
			text:     "> ",
			expected: AgentIdle,
		},
		{
			name:     "bare > in output is not idle",
			text:     "output > file.txt",
			expected: AgentRunning,
		},
		{
			name:     "quoted > in email is not idle",
			text:     "> quoted email text",
			expected: AgentRunning,
		},
		{
			name:     "unknown defaults to running",
			text:     "some random output",
			expected: AgentRunning,
		},
		{
			name:     "empty string defaults to running",
			text:     "",
			expected: AgentRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferStatus(tt.text)
			if result != tt.expected {
				t.Errorf("inferStatus(%q) = %v, want %v", tt.text, result.Label(), tt.expected.Label())
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's a test", "'it'\\''s a test'"},
		{"$(whoami)", "'$(whoami)'"},
		{"`id`", "'`id`'"},
		{"a; rm -rf /", "'a; rm -rf /'"},
		{"foo\nbar", "'foo bar'"},
		{"foo\rbar", "'foo bar'"},
		{"a | b & c", "'a | b & c'"},
		{"", "''"},
	}

	for _, tt := range tests {
		result := shellQuote(tt.input)
		if result != tt.expected {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPaneManagerActiveCount(t *testing.T) {
	pm := &PaneManager{}

	if count := pm.ActiveCount(); count != 0 {
		t.Errorf("empty PaneManager.ActiveCount() = %d, want 0", count)
	}

	pm.slots[0] = &WorktreeSlot{Index: 0, SurfaceID: "s1"}
	if count := pm.ActiveCount(); count != 1 {
		t.Errorf("one slot PaneManager.ActiveCount() = %d, want 1", count)
	}

	pm.slots[2] = &WorktreeSlot{Index: 2, SurfaceID: "s3"}
	if count := pm.ActiveCount(); count != 2 {
		t.Errorf("two slots PaneManager.ActiveCount() = %d, want 2", count)
	}

	pm.slots[1] = &WorktreeSlot{Index: 1, SurfaceID: "s2"}
	if count := pm.ActiveCount(); count != 3 {
		t.Errorf("full PaneManager.ActiveCount() = %d, want 3", count)
	}
}

func TestPaneManagerSlots(t *testing.T) {
	pm := &PaneManager{}
	pm.slots[0] = &WorktreeSlot{
		Index:     0,
		SurfaceID: "surface-abc",
		Issue:     Issue{Identifier: "TEST-123", Title: "Fix bug"},
		Status:    AgentRunning,
	}

	slots := pm.Slots()
	if slots[0] == nil {
		t.Fatal("slot 0 should not be nil")
	}
	if slots[0].Issue.Identifier != "TEST-123" {
		t.Errorf("slot 0 identifier = %s, want TEST-123", slots[0].Issue.Identifier)
	}
	if slots[1] != nil {
		t.Error("slot 1 should be nil")
	}
	if slots[2] != nil {
		t.Error("slot 2 should be nil")
	}
}

func TestPaneManagerLayout(t *testing.T) {
	// 3-slot = stacked
	pm3 := NewPaneManager(NewCmuxClient(), 3)
	if pm3.layout != LayoutStacked {
		t.Errorf("3-slot layout = %d, want LayoutStacked", pm3.layout)
	}
	if pm3.maxSlots != 3 {
		t.Errorf("3-slot maxSlots = %d, want 3", pm3.maxSlots)
	}

	// 4-slot = grid
	pm4 := NewPaneManager(NewCmuxClient(), 4)
	if pm4.layout != LayoutGrid {
		t.Errorf("4-slot layout = %d, want LayoutGrid", pm4.layout)
	}

	// 2-slot = stacked
	pm2 := NewPaneManager(NewCmuxClient(), 2)
	if pm2.layout != LayoutStacked {
		t.Errorf("2-slot layout = %d, want LayoutStacked", pm2.layout)
	}
	if pm2.maxSlots != 2 {
		t.Errorf("2-slot maxSlots = %d, want 2", pm2.maxSlots)
	}

	// Invalid defaults to 3
	pmBad := NewPaneManager(NewCmuxClient(), 99)
	if pmBad.maxSlots != 3 {
		t.Errorf("invalid maxSlots should default to 3, got %d", pmBad.maxSlots)
	}
}

func TestSplitStrategyStacked(t *testing.T) {
	pm := NewPaneManager(NewCmuxClient(), 3)
	pm.tuiSurface = "tui-surface"

	// Slot 0: split TUI right
	target, dir := pm.splitStrategy(0)
	if target != "tui-surface" || dir != "right" {
		t.Errorf("slot 0: got target=%q dir=%q, want tui-surface/right", target, dir)
	}

	// Simulate slot 0 occupied
	pm.slots[0] = &WorktreeSlot{SurfaceID: "s0"}

	// Slot 1: split slot 0 down
	target, dir = pm.splitStrategy(1)
	if target != "s0" || dir != "down" {
		t.Errorf("slot 1: got target=%q dir=%q, want s0/down", target, dir)
	}

	// Simulate slot 1 occupied
	pm.slots[1] = &WorktreeSlot{SurfaceID: "s1"}

	// Slot 2: split slot 1 down
	target, dir = pm.splitStrategy(2)
	if target != "s1" || dir != "down" {
		t.Errorf("slot 2: got target=%q dir=%q, want s1/down", target, dir)
	}
}

func TestSplitStrategyGrid(t *testing.T) {
	pm := NewPaneManager(NewCmuxClient(), 4)
	pm.tuiSurface = "tui-surface"

	// Slot 0: split TUI right
	target, dir := pm.splitStrategy(0)
	if target != "tui-surface" || dir != "right" {
		t.Errorf("slot 0: got target=%q dir=%q, want tui-surface/right", target, dir)
	}

	pm.slots[0] = &WorktreeSlot{SurfaceID: "s0"}

	// Slot 1: split slot 0 down
	target, dir = pm.splitStrategy(1)
	if target != "s0" || dir != "down" {
		t.Errorf("slot 1: got target=%q dir=%q, want s0/down", target, dir)
	}

	pm.slots[1] = &WorktreeSlot{SurfaceID: "s1"}

	// Slot 2: split slot 0 right (top row gets 2 panes)
	target, dir = pm.splitStrategy(2)
	if target != "s0" || dir != "right" {
		t.Errorf("slot 2: got target=%q dir=%q, want s0/right", target, dir)
	}

	// Slot 3: split slot 1 right (bottom row gets 2 panes)
	target, dir = pm.splitStrategy(3)
	if target != "s1" || dir != "right" {
		t.Errorf("slot 3: got target=%q dir=%q, want s1/right", target, dir)
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello world", "world", "foo") {
		t.Error("should find 'world' in 'hello world'")
	}
	if containsAny("hello world", "foo", "bar") {
		t.Error("should not find 'foo' or 'bar' in 'hello world'")
	}
	if !containsAny("[y/n] proceed?", "[y/n]") {
		t.Error("should find '[y/n]' in '[y/n] proceed?'")
	}
	if containsAny("", "anything") {
		t.Error("empty string should not contain anything")
	}
}

func TestCmuxIdentify(t *testing.T) {
	tests := []struct {
		name         string
		scriptBody   string
		exitCode     int
		wantErr      bool
		wantWorkspace string
		wantSurface  string
	}{
		{
			name:          "valid identify output",
			scriptBody:    `{"caller":{"workspace_ref":"ws-123","surface_ref":"sf-456"}}`,
			wantWorkspace: "ws-123",
			wantSurface:   "sf-456",
		},
		{
			name:       "missing caller field",
			scriptBody: `{"other":"data"}`,
			wantErr:    true,
		},
		{
			name:       "malformed json",
			scriptBody: `not json {{{`,
			wantErr:    true,
		},
		{
			name:       "non-zero exit",
			scriptBody: `{"caller":{"workspace_ref":"ws","surface_ref":"sf"}}`,
			exitCode:   1,
			wantErr:    true,
		},
		{
			name:          "extra fields ignored",
			scriptBody:    `{"caller":{"workspace_ref":"w","surface_ref":"s","extra":"ignored"},"version":"1.0"}`,
			wantWorkspace: "w",
			wantSurface:   "s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeCmux(t, tt.scriptBody, tt.exitCode)
			result, err := cmuxIdentify()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%+v)", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.workspaceRef != tt.wantWorkspace {
				t.Errorf("workspaceRef = %q, want %q", result.workspaceRef, tt.wantWorkspace)
			}
			if result.surfaceRef != tt.wantSurface {
				t.Errorf("surfaceRef = %q, want %q", result.surfaceRef, tt.wantSurface)
			}
		})
	}
}

func TestCmuxIdentifyNotOnPath(t *testing.T) {
	// Empty PATH: cmux lookup should fail.
	t.Setenv("PATH", t.TempDir())
	if _, err := cmuxIdentify(); err == nil {
		t.Fatal("expected error when cmux not on PATH")
	}
}

func TestStatusReadLinesConstant(t *testing.T) {
	// The window must be wide enough to catch Claude Code prompts that
	// scroll above the last few lines (see PR #41). 20 is the current value;
	// dropping below 20 should be a deliberate decision.
	if statusReadLines < 20 {
		t.Errorf("statusReadLines = %d, must be >= 20 to catch scrolled prompts", statusReadLines)
	}
}

func TestInferStatusScrolledPrompt(t *testing.T) {
	// When Claude Code's prompt scrolls above the last few lines, the 20-line
	// read window must still include it. Simulate a buffer where the prompt
	// appears several lines above the bottom.
	var lines []string
	lines = append(lines, "❯ finished editing file.go")
	for i := 0; i < 15; i++ {
		lines = append(lines, fmt.Sprintf("output line %d", i))
	}
	text := strings.Join(lines, "\n")

	// inferStatus should still detect the ❯ character anywhere in the blob.
	if got := inferStatus(text); got != AgentIdle {
		t.Errorf("inferStatus with scrolled ❯ = %v, want AgentIdle", got.Label())
	}
}
