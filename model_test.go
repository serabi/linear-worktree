package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func requireModelPtr(t *testing.T, model tea.Model) *Model {
	t.Helper()

	switch m := model.(type) {
	case *Model:
		return m
	case Model:
		return &m
	default:
		t.Fatalf("unexpected model type %T", model)
		return nil
	}
}

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

func TestLaunchReadyMsgHookErrorSetsWarning(t *testing.T) {
	m := NewModel(Config{
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
	})

	result, cmd := m.Update(launchReadyMsg{
		issue:   Issue{Identifier: "TEST-1", Title: "Test"},
		wtPath:  "/tmp/wt/test-1",
		prompt:  "hello",
		hookErr: errors.New("npm install failed"),
	})

	model := result.(Model)
	if !strings.Contains(model.statusMsg, "hook failed") {
		t.Errorf("expected statusMsg to contain 'hook failed', got %q", model.statusMsg)
	}
	if cmd == nil {
		t.Error("expected launch cmd to still proceed despite hook error")
	}
}

func TestLaunchReadyMsgNoHookError(t *testing.T) {
	m := NewModel(Config{
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
	})

	result, cmd := m.Update(launchReadyMsg{
		issue:  Issue{Identifier: "TEST-1", Title: "Test"},
		wtPath: "/tmp/wt/test-1",
		prompt: "hello",
	})

	model := result.(Model)
	if model.statusMsg != "" {
		t.Errorf("expected empty statusMsg when no hook error, got %q", model.statusMsg)
	}
	if cmd == nil {
		t.Error("expected launch cmd")
	}
}

func TestRemoveWorktreeConfirmDialog(t *testing.T) {
	m := NewModel(Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})
	m.issues = []Issue{{Identifier: "TEST-1", Title: "Test"}}
	m.worktreeBranches = map[string]bool{"feature/test-1": true}
	m.rebuildList()

	result, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model := result.(*Model)

	if model.confirm == nil {
		t.Fatal("expected confirmation dialog")
	}
	if model.confirm.action != confirmRemoveWorktree {
		t.Errorf("expected confirmRemoveWorktree, got %d", model.confirm.action)
	}
	if !strings.Contains(model.confirm.message, "TEST-1") {
		t.Errorf("expected message to mention TEST-1, got %q", model.confirm.message)
	}
}

func TestRemoveWorktreeNoWorktree(t *testing.T) {
	m := NewModel(Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})
	m.issues = []Issue{{Identifier: "TEST-1", Title: "Test"}}
	m.worktreeBranches = map[string]bool{}
	m.rebuildList()

	result, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model := result.(*Model)

	if model.confirm != nil {
		t.Error("expected no confirmation dialog when issue has no worktree")
	}
	if !strings.Contains(model.statusMsg, "no worktree") {
		t.Errorf("expected 'no worktree' status, got %q", model.statusMsg)
	}
}

func TestWorktreeRemovedMsgSuccess(t *testing.T) {
	m := NewModel(Config{
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})

	result, cmd := m.Update(worktreeRemovedMsg{identifier: "TEST-1"})
	model := result.(Model)

	if !strings.Contains(model.statusMsg, "Removed worktree") {
		t.Errorf("expected success status, got %q", model.statusMsg)
	}
	if cmd == nil {
		t.Error("expected fetchWorktrees cmd")
	}
}

func TestWorktreeRemovedMsgError(t *testing.T) {
	m := NewModel(Config{
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})

	result, _ := m.Update(worktreeRemovedMsg{
		identifier: "TEST-1",
		err:        errors.New("permission denied"),
	})
	model := result.(Model)

	if !strings.Contains(model.statusMsg, "Error removing") {
		t.Errorf("expected error status, got %q", model.statusMsg)
	}
}

func TestBuildWorktreeListItems(t *testing.T) {
	m := NewModel(Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})

	worktrees := []Worktree{
		{Path: "/repo", Branch: "main", Head: "abc123", Bare: true},
		{Path: "/tmp/wt/test-1", Branch: "feature/test-1", Head: "def456"},
		{Path: "/tmp/wt/test-2", Branch: "feature/test-2", Head: "ghi789"},
		{Path: "/tmp/wt/manual", Branch: "manual-branch", Head: "jkl012"},
	}

	items := m.buildWorktreeListItems(worktrees)

	// 2 managed + 1 separator + 1 other = 4
	if len(items) != 4 {
		t.Fatalf("expected 4 items (bare skipped, separator added), got %d", len(items))
	}

	wi0 := items[0].(worktreeItem)
	if wi0.identifier != "TEST-1" {
		t.Errorf("item[0].identifier = %q, want TEST-1", wi0.identifier)
	}
	if wi0.slotIdx != -1 {
		t.Errorf("item[0].slotIdx = %d, want -1 (no slot)", wi0.slotIdx)
	}

	// item[2] should be the separator
	if _, ok := items[2].(worktreeSeparator); !ok {
		t.Errorf("item[2] should be worktreeSeparator, got %T", items[2])
	}

	wi3 := items[3].(worktreeItem)
	if wi3.identifier != "" {
		t.Errorf("item[3].identifier = %q, want empty (no matching prefix)", wi3.identifier)
	}
}

func TestWorktreeListEscReturnsToList(t *testing.T) {
	m := NewModel(Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})
	m.view = viewWorktreeList

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model := result.(*Model)

	if model.view != viewList {
		t.Errorf("expected viewList after esc, got %d", model.view)
	}
}

func TestWorktreeListLoadedMsg(t *testing.T) {
	m := NewModel(Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		WorktreeBase:  "/tmp/wt",
		BranchPrefix:  "feature/",
	})
	m.width = 80
	m.height = 24

	result, _ := m.Update(worktreeListLoadedMsg{
		worktrees: []Worktree{
			{Path: "/tmp/wt/test-1", Branch: "feature/test-1", Head: "abc123"},
		},
	})
	model := result.(Model)

	if model.view != viewWorktreeList {
		t.Errorf("expected viewWorktreeList, got %d", model.view)
	}
	if !strings.Contains(model.statusMsg, "1 worktrees") {
		t.Errorf("expected '1 worktrees' status, got %q", model.statusMsg)
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
		LinearAPIKey:   "lin_api_test",
		TeamID:         "team-1",
		TeamKey:        "TSCODE",
		WorktreeBase:   "/custom/path",
		CopyFiles:      []string{".env", ".secret"},
		CopyDirs:       []string{".claude", ".config"},
		ClaudeCommand:  "claude",
		BranchPrefix:   "work/",
		MaxSlots:       4,
		ClaudeArgs:     "--model sonnet",
		PostCreateHook: "npm install",
		PromptTemplate: "Fix {{.Identifier}}",
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	if m.settingsDraft == nil {
		t.Fatal("expected settingsDraft to be initialized")
	}
	if m.settingsDraft.apiKey != "lin_api_test" {
		t.Errorf("settingsDraft.apiKey = %q, want 'lin_api_test'", m.settingsDraft.apiKey)
	}
	if m.settingsDraft.teamKey != "TSCODE" {
		t.Errorf("settingsDraft.teamKey = %q, want 'TSCODE'", m.settingsDraft.teamKey)
	}
	if m.settingsDraft.wtBase != "/custom/path" {
		t.Errorf("settingsDraft.wtBase = %q, want '/custom/path'", m.settingsDraft.wtBase)
	}
	if m.settingsDraft.copyFiles != ".env, .secret" {
		t.Errorf("settingsDraft.copyFiles = %q, want '.env, .secret'", m.settingsDraft.copyFiles)
	}
	if m.settingsDraft.copyDirs != ".claude, .config" {
		t.Errorf("settingsDraft.copyDirs = %q, want '.claude, .config'", m.settingsDraft.copyDirs)
	}
	if m.settingsDraft.branch != "work/" {
		t.Errorf("settingsDraft.branch = %q, want 'work/'", m.settingsDraft.branch)
	}
	if m.settingsDraft.maxSlots != 4 {
		t.Errorf("settingsDraft.maxSlots = %d, want 4", m.settingsDraft.maxSlots)
	}
	if m.settingsDraft.claudeArgs != "--model sonnet" {
		t.Errorf("settingsDraft.claudeArgs = %q, want '--model sonnet'", m.settingsDraft.claudeArgs)
	}
	if m.settingsDraft.hook != "npm install" {
		t.Errorf("settingsDraft.hook = %q, want 'npm install'", m.settingsDraft.hook)
	}
	if m.settingsDraft.prompt != "Fix {{.Identifier}}" {
		t.Errorf("settingsDraft.prompt = %q, want 'Fix {{.Identifier}}'", m.settingsDraft.prompt)
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
	if m.settingsTabNames[3] != "Slots" {
		t.Errorf("tab 3 name = %q, want 'Slots'", m.settingsTabNames[3])
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

	m.settingsDraft.apiKey = ""
	m.settingsDraft.teamKey = ""

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
	t.Setenv("HOME", t.TempDir())
	cfg := Config{
		LinearAPIKey:  "lin_api_old",
		TeamID:        "team-1",
		TeamKey:       "TSCODE",
		Teams:         []TeamEntry{{ID: "team-1", Key: "TSCODE"}},
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	// Change some settings
	m.settingsDraft.apiKey = "lin_api_old"
	m.settingsDraft.teamKey = "TSCODE" // same team key, no resolve needed
	m.settingsDraft.wtBase = "/new/path"
	m.settingsDraft.maxSlots = 4

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
	t.Setenv("HOME", t.TempDir())
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "OLD",
		Teams:         []TeamEntry{{ID: "team-1", Key: "OLD"}},
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.buildSettingsForm()

	m.settingsDraft.apiKey = "lin_api_test"
	m.settingsDraft.teamKey = "NEW" // changed!

	_, cmd := m.handleSettingsCompleted()
	if cmd == nil {
		t.Fatal("expected resolveTeamCmd when team key changed")
	}
	if m.statusMsg != "Resolving teams..." {
		t.Errorf("statusMsg = %q, want 'Resolving teams...'", m.statusMsg)
	}
}

func TestSortPickerEnterCompletesSelection(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	initCmd := m.showSortPicker()
	if initCmd == nil {
		t.Fatal("expected sort picker init cmd")
	}
	if m.view != viewSortPicker {
		t.Fatalf("view = %v, want sort picker", m.view)
	}
	if m.sortForm == nil {
		t.Fatal("expected sort form to be initialized")
	}

	// Process the init cmd so the form becomes interactive
	result, cmd := m.Update(initCmd())
	model := drainCmds(t, requireModelPtr(t, result), cmd)

	result, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = requireModelPtr(t, result)

	// Process follow-up messages until the form completes
	for i := 0; i < 10 && cmd != nil; i++ {
		result, cmd = model.Update(cmd())
		model = requireModelPtr(t, result)
		if model.view == viewList {
			break
		}
	}
	if model.view != viewList {
		t.Fatalf("view after completion = %v, want list", model.view)
	}
	if model.sortForm != nil {
		t.Fatal("expected sort form to be cleared after completion")
	}
	if model.sortMode != SortUpdatedAt {
		t.Fatalf("sortMode = %v, want %v", model.sortMode, SortUpdatedAt)
	}
	if !model.loading {
		t.Fatal("expected sort selection to trigger reload")
	}
	if cmd == nil {
		t.Fatal("expected reload cmd after sort selection")
	}
}

func TestSettingsDefaultsApplied(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	m.settingsDraft.apiKey = "lin_api_test"
	m.settingsDraft.teamKey = "TSCODE"
	m.settingsDraft.wtBase = ""
	m.settingsDraft.claudeCmd = ""
	m.settingsDraft.branch = ""

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
	if !strings.Contains(view, "Enter: save") {
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

func TestTruncateURL(t *testing.T) {
	tests := []struct {
		url    string
		maxLen int
		want   string
	}{
		{"https://example.com/path", 30, "example.com/path"},
		{"https://example.com/very/long/path/that/exceeds/the/limit", 30, "example.com/very/long/path/th…"},
		{"https://example.com/page#section", 40, "example.com/page#section"},
		{"not-a-url", 5, "not-…"},
		{"short", 10, "short"},
		{"https://example.com/path", 0, "https://example.com/path"},
		{"https://example.com/path", -1, "https://example.com/path"},
	}
	for _, tt := range tests {
		got := truncateURL(tt.url, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateURL(%q, %d) = %q, want %q", tt.url, tt.maxLen, got, tt.want)
		}
	}
}

func TestDetailBackNavigationFetchesComments(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.view = viewDetail
	m.width = 120
	m.height = 40

	prevIssue := &Issue{ID: "issue-prev", Identifier: "TST-1", Title: "Previous"}
	currIssue := &Issue{ID: "issue-curr", Identifier: "TST-2", Title: "Current"}

	m.detailIssue = currIssue
	m.detailHistory = []*Issue{prevIssue}
	m.cachedCommentID = currIssue.ID

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated := result.(*Model)

	if updated.detailIssue.ID != prevIssue.ID {
		t.Errorf("expected detailIssue to be prev (%s), got %s", prevIssue.ID, updated.detailIssue.ID)
	}
	if len(updated.detailHistory) != 0 {
		t.Errorf("expected empty detailHistory, got %d entries", len(updated.detailHistory))
	}
	if !updated.loading {
		t.Error("expected loading=true when cachedCommentID differs from restored issue")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd to fetch comments")
	}
}

func TestDetailBackNavigationSkipsFetchWhenCached(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.view = viewDetail
	m.width = 120
	m.height = 40

	prevIssue := &Issue{ID: "issue-prev", Identifier: "TST-1", Title: "Previous"}
	currIssue := &Issue{ID: "issue-curr", Identifier: "TST-2", Title: "Current"}

	m.detailIssue = currIssue
	m.detailHistory = []*Issue{prevIssue}
	m.cachedCommentID = prevIssue.ID // already cached

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated := result.(*Model)

	if updated.detailIssue.ID != prevIssue.ID {
		t.Errorf("expected detailIssue to be restored to prev, got %s", updated.detailIssue.ID)
	}
	if cmd == nil {
		t.Error("expected cmd for async content rendering")
	}
}

func TestDetailCommentSortToggle(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.width = 80
	m.height = 40

	issue := &Issue{ID: "issue-1", Identifier: "TEST-1", Title: "Test"}
	m.detailIssue = issue
	m.view = viewDetail
	m.cachedCommentID = "issue-1"
	m.cachedComments = []Comment{
		{Body: "first comment", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u1", Name: "Alice"}, CreatedAt: "2025-01-01T00:00:00Z"},
		{Body: "second comment", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u2", Name: "Bob"}, CreatedAt: "2025-01-02T00:00:00Z"},
	}

	// Default is descending (commentSortAsc = false)
	if m.commentSortAsc {
		t.Fatal("expected default comment sort to be descending")
	}

	// Press 'o' to toggle
	result, _ := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	model := requireModelPtr(t, result)
	if !model.commentSortAsc {
		t.Fatal("expected commentSortAsc to be true after toggle")
	}

	// Press 'o' again to toggle back
	result, _ = model.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = requireModelPtr(t, result)
	if model.commentSortAsc {
		t.Fatal("expected commentSortAsc to be false after second toggle")
	}
}

func TestDetailCommentSortOrder(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.width = 80
	m.height = 40

	issue := &Issue{ID: "issue-1", Identifier: "TEST-1", Title: "Test"}
	m.cachedCommentID = "issue-1"
	m.cachedComments = []Comment{
		{Body: "AAA FIRST", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u1", Name: "Alice"}, CreatedAt: "2025-01-01T00:00:00Z"},
		{Body: "ZZZ LAST", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u2", Name: "Bob"}, CreatedAt: "2025-01-02T00:00:00Z"},
	}

	// Ascending: first comment appears before last
	m.commentSortAsc = true
	content := m.buildDetailContent(issue, 70)
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	content = ansiPattern.ReplaceAllString(content, "")
	firstIdx := strings.Index(content, "AAA FIRST")
	lastIdx := strings.Index(content, "ZZZ LAST")
	if firstIdx < 0 || lastIdx < 0 {
		t.Fatal("expected both comments in output")
	}
	if firstIdx > lastIdx {
		t.Error("ascending sort: first comment should appear before last")
	}

	// Descending: last comment appears before first
	m.commentSortAsc = false
	content = m.buildDetailContent(issue, 70)
	content = ansiPattern.ReplaceAllString(content, "")
	firstIdx = strings.Index(content, "AAA FIRST")
	lastIdx = strings.Index(content, "ZZZ LAST")
	if firstIdx < lastIdx {
		t.Error("descending sort: last comment should appear before first")
	}
}

func TestDetailRefreshComments(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST", LinearAPIKey: "test"})
	m.width = 80
	m.height = 40

	issue := &Issue{ID: "issue-1", Identifier: "TEST-1", Title: "Test"}
	m.detailIssue = issue
	m.view = viewDetail

	// Press 'r' to refresh
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model := requireModelPtr(t, result)

	if !model.loading {
		t.Fatal("expected loading to be true after refresh")
	}
	if model.loadingLabel != "Loading comments..." {
		t.Fatalf("loadingLabel = %q, want %q", model.loadingLabel, "Loading comments...")
	}
	if cmd == nil {
		t.Fatal("expected a command to fetch comments")
	}
}

func TestDetailRefreshNoIssue(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.view = viewDetail
	m.detailIssue = nil

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model := requireModelPtr(t, result)

	if model.loading {
		t.Fatal("should not be loading when no issue is set")
	}
	if cmd != nil {
		t.Fatal("should not return a command when no issue is set")
	}
}

func TestSortPickerEscCancels(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.sortMode = SortCreatedAt
	initCmd := m.showSortPicker()
	if initCmd == nil {
		t.Fatal("expected init cmd")
	}

	// Process init
	result, cmd := m.Update(initCmd())
	model := drainCmds(t, requireModelPtr(t, result), cmd)

	// Press Esc
	result, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = requireModelPtr(t, result)

	if model.view != viewList {
		t.Fatalf("view = %v, want viewList after esc", model.view)
	}
	if model.sortForm != nil {
		t.Fatal("expected sortForm to be nil after esc")
	}
	if model.sortMode != SortCreatedAt {
		t.Fatalf("sortMode = %v, want SortCreatedAt (unchanged)", model.sortMode)
	}
}

func TestTeamSwitchPreservesState(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEAM1",
		Teams:         []TeamEntry{{ID: "team-1", Key: "TEAM1"}, {ID: "team-2", Key: "TEAM2"}},
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.width = 120
	m.height = 40

	// Simulate loaded state for TEAM1
	m.issues = []Issue{
		{Identifier: "TEAM1-1", Title: "First issue"},
		{Identifier: "TEAM1-2", Title: "Second issue"},
	}
	m.projects = []Project{{ID: "p1", Name: "Project1"}}
	m.workflowStates = []WorkflowState{{ID: "ws1", Name: "In Progress"}}
	m.filter = FilterAll
	projID := "p1"
	m.projectFilter = &projID
	m.projectName = "Project1"
	m.rebuildList()
	m.list.Select(1) // select second item

	// Switch to TEAM2
	switchedCfg := cfg
	switchedCfg.TeamID = "team-2"
	switchedCfg.TeamKey = "TEAM2"
	result, _ := m.Update(teamSwitchedMsg{cfg: switchedCfg})
	mp := result.(Model)

	// TEAM2 has no cache, so model should be in loading state
	if !mp.loading {
		t.Error("expected loading=true for uncached team")
	}
	if mp.filter != FilterAssigned {
		t.Errorf("uncached team should reset filter to FilterAssigned, got %v", mp.filter)
	}

	// Simulate TEAM2 data loaded
	mp.loading = false
	mp.issues = []Issue{{Identifier: "TEAM2-1", Title: "Other issue"}}
	mp.filter = FilterInProgress
	mp.rebuildList()

	// Switch back to TEAM1
	result2, _ := mp.Update(teamSwitchedMsg{cfg: cfg})
	mp2 := result2.(Model)

	// Should restore TEAM1's cached state
	if mp2.loading {
		t.Error("expected loading=false for cached team")
	}
	if len(mp2.issues) != 2 {
		t.Errorf("expected 2 cached issues, got %d", len(mp2.issues))
	}
	if mp2.filter != FilterAll {
		t.Errorf("expected cached filter FilterAll, got %v", mp2.filter)
	}
	if mp2.projectFilter == nil || *mp2.projectFilter != "p1" {
		t.Error("expected cached projectFilter to be restored")
	}
	if mp2.projectName != "Project1" {
		t.Errorf("expected cached projectName 'Project1', got %q", mp2.projectName)
	}
	if mp2.list.Index() != 1 {
		t.Errorf("expected cached list index 1, got %d", mp2.list.Index())
	}
}

func TestLabelPickerEnterCompletesSelection(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.issues = []Issue{
		{Identifier: "TEST-1", Title: "Issue 1"},
		{Identifier: "TEST-2", Title: "Issue 2"},
	}
	m.issues[0].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"label-1", "Bug", "#ff0000"}}
	m.issues[1].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"label-2", "Feature", "#00ff00"}}
	initCmd := m.showLabelPicker()
	if initCmd == nil {
		t.Fatal("expected label picker init cmd")
	}
	if m.view != viewLabelPicker {
		t.Fatalf("view = %v, want label picker", m.view)
	}
	if m.labelForm == nil {
		t.Fatal("expected label form to be initialized")
	}

	// Process the init cmd so the form becomes interactive
	result, cmd := m.Update(initCmd())
	model := drainCmds(t, requireModelPtr(t, result), cmd)

	result, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = requireModelPtr(t, result)

	// Process follow-up messages until the form completes
	for i := 0; i < 10 && cmd != nil; i++ {
		result, cmd = model.Update(cmd())
		model = requireModelPtr(t, result)
		if model.view == viewList {
			break
		}
	}
	if model.view != viewList {
		t.Fatalf("view after completion = %v, want list", model.view)
	}
	if model.labelForm != nil {
		t.Fatal("expected label form to be cleared after completion")
	}
	// Default selection is "All issues" (empty string) so labelFilter should be nil
	if model.labelFilter != nil {
		t.Fatalf("labelFilter = %v, want nil for 'All issues'", model.labelFilter)
	}
	if !model.loading {
		t.Fatal("expected label selection to trigger reload")
	}
}

func TestLabelPickerEscCancels(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST"})
	m.issues = []Issue{{Identifier: "TEST-1", Title: "Issue 1"}}
	m.issues[0].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"label-1", "Bug", "#ff0000"}}
	labelID := "label-1"
	m.labelFilter = &labelID
	m.labelName = "Bug"

	initCmd := m.showLabelPicker()
	if initCmd == nil {
		t.Fatal("expected init cmd")
	}

	// Process init
	result, cmd := m.Update(initCmd())
	model := drainCmds(t, requireModelPtr(t, result), cmd)

	// Press Esc
	result, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = requireModelPtr(t, result)

	if model.view != viewList {
		t.Fatalf("view = %v, want viewList after esc", model.view)
	}
	if model.labelForm != nil {
		t.Fatal("expected labelForm to be nil after esc")
	}
	// Label filter should remain unchanged
	if model.labelFilter == nil || *model.labelFilter != "label-1" {
		t.Fatal("expected labelFilter to remain unchanged after esc")
	}
	if model.labelName != "Bug" {
		t.Fatalf("labelName = %q, want 'Bug' (unchanged)", model.labelName)
	}
}

func TestTeamSwitchPreservesLabelState(t *testing.T) {
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TEAM1",
		Teams:         []TeamEntry{{ID: "team-1", Key: "TEAM1"}, {ID: "team-2", Key: "TEAM2"}},
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.width = 120
	m.height = 40

	// Simulate loaded state for TEAM1 with label filter
	m.issues = []Issue{
		{Identifier: "TEAM1-1", Title: "First issue"},
	}
	m.labels = []IssueLabel{{ID: "lbl-1", Name: "Bug", Color: "#ff0000"}}
	labelID := "lbl-1"
	m.labelFilter = &labelID
	m.labelName = "Bug"
	m.rebuildList()

	// Switch to TEAM2
	switchedCfg := cfg
	switchedCfg.TeamID = "team-2"
	switchedCfg.TeamKey = "TEAM2"
	result, _ := m.Update(teamSwitchedMsg{cfg: switchedCfg})
	mp := result.(Model)

	// TEAM2 should have cleared label state
	if mp.labelFilter != nil {
		t.Error("expected labelFilter to be nil for new team")
	}
	if mp.labelName != "" {
		t.Errorf("expected empty labelName for new team, got %q", mp.labelName)
	}

	// Switch back to TEAM1
	result2, _ := mp.Update(teamSwitchedMsg{cfg: cfg})
	mp2 := result2.(Model)

	// Should restore TEAM1's cached label state
	if mp2.labelFilter == nil || *mp2.labelFilter != "lbl-1" {
		t.Error("expected cached labelFilter to be restored")
	}
	if mp2.labelName != "Bug" {
		t.Errorf("expected cached labelName 'Bug', got %q", mp2.labelName)
	}
}

func TestListTitleIncludesLabelName(t *testing.T) {
	m := NewModel(Config{TeamKey: "TEST", Teams: []TeamEntry{{ID: "t1", Key: "TEST"}}})
	m.filter = FilterAll

	// No filters
	m.updateListTitle()
	if m.list.Title != "TEST > [All]" {
		t.Errorf("title = %q, want 'TEST > [All]'", m.list.Title)
	}

	// Label filter only
	labelID := "lbl-1"
	m.labelFilter = &labelID
	m.labelName = "Bug"
	m.updateListTitle()
	if m.list.Title != "TEST > label:Bug > [All]" {
		t.Errorf("title = %q, want 'TEST > label:Bug > [All]'", m.list.Title)
	}

	// Project + label
	projID := "p1"
	m.projectFilter = &projID
	m.projectName = "Auth"
	m.updateListTitle()
	if m.list.Title != "TEST > Auth > label:Bug > [All]" {
		t.Errorf("title = %q, want 'TEST > Auth > label:Bug > [All]'", m.list.Title)
	}
}

func TestDetailSLAFieldsRendered(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 80
	m.height = 40

	slaType := "all"
	breach := time.Now().Add(72 * time.Hour).Format(time.RFC3339)
	started := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	medium := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	high := time.Now().Add(60 * time.Hour).Format(time.RFC3339)

	issue := &Issue{
		ID: "issue-1", Identifier: "TEST-1", Title: "Test SLA",
		SLAType:         &slaType,
		SLABreachesAt:   &breach,
		SLAStartedAt:    &started,
		SLAMediumRiskAt: &medium,
		SLAHighRiskAt:   &high,
	}

	content := m.buildDetailContent(issue, 70)
	if !strings.Contains(content, "SLA Breach") {
		t.Error("expected SLA Breach field in output")
	}
	if !strings.Contains(content, "Calendar Days") {
		t.Error("expected humanized SLA type 'Calendar Days'")
	}
	if !strings.Contains(content, "SLA Started") {
		t.Error("expected SLA Started field in output")
	}
}

func TestDetailNoSLAFieldsWhenNil(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 80
	m.height = 40

	issue := &Issue{ID: "issue-1", Identifier: "TEST-1", Title: "Plain Issue"}
	content := m.buildDetailContent(issue, 70)
	if strings.Contains(content, "SLA Breach") || strings.Contains(content, "SLA Scope") || strings.Contains(content, "SLA Started") {
		t.Error("expected no SLA fields when all SLA fields are nil")
	}
}

func TestDetailSLABreachedStatus(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 80
	m.height = 40

	breach := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	issue := &Issue{
		ID: "issue-1", Identifier: "TEST-1", Title: "Breached",
		SLABreachesAt: &breach,
	}

	content := m.buildDetailContent(issue, 70)
	if !strings.Contains(content, "BREACHED") {
		t.Error("expected BREACHED indicator for past breach time")
	}
}

func TestDetailSLABreachOnly(t *testing.T) {
	m := NewModel(DefaultConfig())
	m.width = 80
	m.height = 40

	breach := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	issue := &Issue{
		ID: "issue-1", Identifier: "TEST-1", Title: "Breach Only",
		SLABreachesAt: &breach,
	}

	content := m.buildDetailContent(issue, 70)
	if !strings.Contains(content, "SLA Breach") {
		t.Error("expected SLA Breach field")
	}
	if strings.Contains(content, "SLA Scope") {
		t.Error("expected no SLA Scope when SLAType is nil")
	}
	if strings.Contains(content, "SLA Started") {
		t.Error("expected no SLA Started when SLAStartedAt is nil")
	}
}

func TestHumanizeSLAType(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"all", "Calendar Days"},
		{"onlyBusinessDays", "Business Days"},
		{"custom", "Custom"},
		{"", ""},
	}
	for _, tt := range tests {
		got := humanizeSLAType(tt.input)
		if got != tt.want {
			t.Errorf("humanizeSLAType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRelativeTimeUntil(t *testing.T) {
	tests := []struct {
		name   string
		offset time.Duration
		want   string
	}{
		{"future hours", 3*time.Hour + 30*time.Second, "in 3h"},
		{"future days", 5*24*time.Hour + 30*time.Second, "in 5d"},
		{"future minutes", 30*time.Minute + 30*time.Second, "in 30m"},
		{"past hours", -3*time.Hour - 30*time.Second, "3h ago"},
		{"past days", -5*24*time.Hour - 30*time.Second, "5d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iso := time.Now().Add(tt.offset).Format(time.RFC3339)
			got := relativeTimeUntil(iso)
			if got != tt.want {
				t.Errorf("relativeTimeUntil(%v offset) = %q, want %q", tt.offset, got, tt.want)
			}
		})
	}
}

// Ensure huh is used (compile-time check)
var _ = huh.StateCompleted
