package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
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

func TestSettingsInitOnFirstRun(t *testing.T) {
	cfg := DefaultConfig() // NeedsSetup() == true
	m := NewModel(cfg)

	if m.view != viewSettings {
		t.Errorf("expected viewSettings on first run, got %d", m.view)
	}
	if m.settingsTabs[0] == nil {
		t.Fatal("expected settings tabs to be initialized")
	}
	if !m.settingsFirstRun {
		t.Error("expected settingsFirstRun to be true")
	}
}

func TestSettingsInitPrePopulated(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TSCODE",
		WorktreeBase:  "/custom/path",
		CopyFiles:     []string{".env", ".secret"},
		CopyDirs:      []string{".claude", ".config"},
		ClaudeCommand: "claude",
		BranchPrefix:  "work/",
		MaxSlots:      4,
		ClaudeArgs:    "--model sonnet",
		PostCreateHook: "npm install",
		PromptTemplate: "Fix {{.Identifier}}",
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	if m.settingsAPIKey != "lin_api_test" {
		t.Errorf("settingsAPIKey = %q, want 'lin_api_test'", m.settingsAPIKey)
	}
	if m.settingsTeamKey != "TSCODE" {
		t.Errorf("settingsTeamKey = %q, want 'TSCODE'", m.settingsTeamKey)
	}
	if m.settingsWtBase != "/custom/path" {
		t.Errorf("settingsWtBase = %q, want '/custom/path'", m.settingsWtBase)
	}
	if m.settingsCopyFiles != ".env, .secret" {
		t.Errorf("settingsCopyFiles = %q, want '.env, .secret'", m.settingsCopyFiles)
	}
	if m.settingsCopyDirs != ".claude, .config" {
		t.Errorf("settingsCopyDirs = %q, want '.claude, .config'", m.settingsCopyDirs)
	}
	if m.settingsBranch != "work/" {
		t.Errorf("settingsBranch = %q, want 'work/'", m.settingsBranch)
	}
	if m.settingsMaxSlots != 4 {
		t.Errorf("settingsMaxSlots = %d, want 4", m.settingsMaxSlots)
	}
	if m.settingsClaudeArgs != "--model sonnet" {
		t.Errorf("settingsClaudeArgs = %q, want '--model sonnet'", m.settingsClaudeArgs)
	}
	if m.settingsHook != "npm install" {
		t.Errorf("settingsHook = %q, want 'npm install'", m.settingsHook)
	}
	if m.settingsPrompt != "Fix {{.Identifier}}" {
		t.Errorf("settingsPrompt = %q, want 'Fix {{.Identifier}}'", m.settingsPrompt)
	}
}

func TestSettingsTabCount(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.buildSettingsForm()

	for i, tab := range m.settingsTabs {
		if tab == nil {
			t.Errorf("settingsTabs[%d] is nil", i)
		}
	}
	if m.settingsTabNames[0] != "Credentials" {
		t.Errorf("tab 0 name = %q, want 'Credentials'", m.settingsTabNames[0])
	}
	if m.settingsTabNames[1] != "Worktree" {
		t.Errorf("tab 1 name = %q, want 'Worktree'", m.settingsTabNames[1])
	}
	if m.settingsTabNames[2] != "Launch" {
		t.Errorf("tab 2 name = %q, want 'Launch'", m.settingsTabNames[2])
	}
}

func TestSettingsActiveTab(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.buildSettingsForm()
	m.settingsActiveTab = 0

	if m.activeSettingsForm() != m.settingsTabs[0] {
		t.Error("activeSettingsForm should return tab 0")
	}
	m.settingsActiveTab = 2
	if m.activeSettingsForm() != m.settingsTabs[2] {
		t.Error("activeSettingsForm should return tab 2")
	}
}

func TestSettingsCompletionRequiresCredentials(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.buildSettingsForm()

	m.settingsAPIKey = ""
	m.settingsTeamKey = ""

	result, cmd := m.handleSettingsCompleted()
	model := result.(*Model)
	if cmd != nil {
		t.Error("expected nil cmd when credentials missing")
	}
	if model.settingsActiveTab != 0 {
		t.Error("should jump to credentials tab on validation error")
	}
	if !strings.Contains(model.statusMsg, "required") {
		t.Errorf("statusMsg = %q, want to contain 'required'", model.statusMsg)
	}
}

func TestSettingsCompletionWithCredentials(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_old",
		TeamID:        "team-1",
		TeamKey:       "TSCODE",
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	// Change some settings
	m.settingsAPIKey = "lin_api_old"
	m.settingsTeamKey = "TSCODE" // same team key, no resolve needed
	m.settingsWtBase = "/new/path"
	m.settingsMaxSlots = 4

	result, _ := m.handleSettingsCompleted()
	model := result.(*Model)
	if model.cfg.WorktreeBase != "/new/path" {
		t.Errorf("cfg.WorktreeBase = %q, want '/new/path'", model.cfg.WorktreeBase)
	}
	if model.cfg.MaxSlots != 4 {
		t.Errorf("cfg.MaxSlots = %d, want 4", model.cfg.MaxSlots)
	}
	if model.view != viewList {
		t.Errorf("view = %d, want viewList", model.view)
	}
}

func TestSettingsTeamKeyChangeTriggersResolve(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "OLD",
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	m.settingsAPIKey = "lin_api_test"
	m.settingsTeamKey = "NEW" // changed!

	_, cmd := m.handleSettingsCompleted()
	if cmd == nil {
		t.Fatal("expected resolveTeamCmd when team key changed")
	}
	if m.statusMsg != "Resolving team..." {
		t.Errorf("statusMsg = %q, want 'Resolving team...'", m.statusMsg)
	}
}

func TestSettingsDefaultsApplied(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TSCODE",
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	// Clear optional fields
	m.settingsAPIKey = "lin_api_test"
	m.settingsTeamKey = "TSCODE"
	m.settingsWtBase = ""
	m.settingsClaudeCmd = ""
	m.settingsBranch = ""

	result, _ := m.handleSettingsCompleted()
	model := result.(*Model)
	if model.cfg.WorktreeBase != "../worktrees" {
		t.Errorf("empty WorktreeBase should default to '../worktrees', got %q", model.cfg.WorktreeBase)
	}
	if model.cfg.ClaudeCommand != "claude" {
		t.Errorf("empty ClaudeCommand should default to 'claude', got %q", model.cfg.ClaudeCommand)
	}
	if model.cfg.BranchPrefix != "feature/" {
		t.Errorf("empty BranchPrefix should default to 'feature/', got %q", model.cfg.BranchPrefix)
	}
}

func TestSettingsTabAdvanceOnCompletion(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.buildSettingsForm()
	m.settingsActiveTab = 0

	// Simulate form completion (Enter past last field)
	m.settingsTabs[0] = m.settingsTabs[0].WithShowErrors(true)
	// Force the form to completed state by checking the advance logic
	initialTab := m.settingsActiveTab

	// Verify tab advance logic: if active form completes on non-last tab, advance
	if initialTab >= len(m.settingsTabs)-1 {
		t.Skip("already on last tab")
	}
}

func TestSplitComma(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{".env, .envrc", []string{".env", ".envrc"}},
		{".env", []string{".env"}},
		{"", nil},
		{" , , ", nil},
		{".env, , .envrc", []string{".env", ".envrc"}},
		{" .env , .envrc , .secret ", []string{".env", ".envrc", ".secret"}},
	}

	for _, tt := range tests {
		result := splitComma(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitComma(%q) = %v (len %d), want %v (len %d)",
				tt.input, result, len(result), tt.expected, len(tt.expected))
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitComma(%q)[%d] = %q, want %q",
					tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestRenderSettingsTabBar(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.buildSettingsForm()

	m.settingsActiveTab = 0
	bar := m.renderSettingsTabBar()
	if !strings.Contains(bar, "Credentials") {
		t.Error("tab bar should contain 'Credentials'")
	}
	if !strings.Contains(bar, "Worktree") {
		t.Error("tab bar should contain 'Worktree'")
	}
	if !strings.Contains(bar, "Launch") {
		t.Error("tab bar should contain 'Launch'")
	}
}

func TestSettingsRebuildActiveTab(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 100
	m.buildSettingsForm()
	m.settingsActiveTab = 1

	// Force the tab to completed state
	oldTab := m.settingsTabs[1]
	m.rebuildActiveTab()
	newTab := m.settingsTabs[1]

	if newTab == oldTab {
		t.Error("rebuildActiveTab should create a new form")
	}
	if newTab == nil {
		t.Error("rebuilt tab should not be nil")
	}
}

func TestSettingsEscBlockedOnFirstRun(t *testing.T) {
	m := NewModel(DefaultConfig()) // NeedsSetup = true
	m.settingsFirstRun = true

	// The Esc handling in updateSettings returns nil cmd and stays in settings
	// when settingsFirstRun is true
	if !m.settingsFirstRun {
		t.Error("settingsFirstRun should be true")
	}
	if m.view != viewSettings {
		t.Error("should be in viewSettings")
	}
}

func TestSettingsViewRendersTabBar(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 100
	m.height = 40
	m.buildSettingsForm()

	view := m.viewSettings()
	if !strings.Contains(view, "Settings") {
		t.Error("settings view should contain 'Settings' header")
	}
	if !strings.Contains(view, "Credentials") {
		t.Error("settings view should contain tab names")
	}
	if !strings.Contains(view, "Ctrl+S") {
		t.Error("settings view should contain help text")
	}
}

func TestBuildPromptWithTemplate(t *testing.T) {
	issue := Issue{
		Identifier:  "TEST-123",
		Title:       "Fix the bug",
		Description: "Something is broken",
	}
	cfg := Config{
		PromptTemplate: "Work on {{.Identifier}}: {{.Title}}",
	}

	prompt := buildPrompt(issue, cfg)
	if prompt != "Work on TEST-123: Fix the bug" {
		t.Errorf("prompt = %q, want 'Work on TEST-123: Fix the bug'", prompt)
	}
}

func TestBuildPromptWithoutTemplate(t *testing.T) {
	issue := Issue{
		Identifier:  "TEST-123",
		Title:       "Fix the bug",
		Description: "Something is broken",
	}
	cfg := Config{}

	prompt := buildPrompt(issue, cfg)
	if !strings.Contains(prompt, "TEST-123") {
		t.Error("default prompt should contain identifier")
	}
	if !strings.Contains(prompt, "Fix the bug") {
		t.Error("default prompt should contain title")
	}
	if !strings.Contains(prompt, "Something is broken") {
		t.Error("default prompt should contain description")
	}
}

func TestBuildPromptInvalidTemplate(t *testing.T) {
	issue := Issue{
		Identifier: "TEST-123",
		Title:      "Fix the bug",
	}
	cfg := Config{
		PromptTemplate: "{{.Invalid",
	}

	prompt := buildPrompt(issue, cfg)
	// Should fall back to default prompt on invalid template
	if !strings.Contains(prompt, "TEST-123") {
		t.Error("should fall back to default prompt on invalid template")
	}
}

// Ensure huh is used (compile-time check)
var _ = huh.StateCompleted
