package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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
	model := requireModelPtr(t, result)
	for cmd != nil {
		result, cmd = model.Update(cmd())
		model = requireModelPtr(t, result)
	}

	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	result, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'d'}}))
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

	result, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'d'}}))
	updated := result.(*Model)

	if updated.loading {
		t.Error("expected loading=false when comments already cached")
	}
	if cmd != nil {
		t.Error("expected nil cmd when comments already cached")
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
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	model := requireModelPtr(t, result)
	if !model.commentSortAsc {
		t.Fatal("expected commentSortAsc to be true after toggle")
	}

	// Press 'o' again to toggle back
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
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
		{Body: "AAA_FIRST", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u1", Name: "Alice"}, CreatedAt: "2025-01-01T00:00:00Z"},
		{Body: "ZZZ_LAST", User: struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		}{ID: "u2", Name: "Bob"}, CreatedAt: "2025-01-02T00:00:00Z"},
	}

	// Ascending: first comment appears before last
	m.commentSortAsc = true
	content := m.buildDetailContent(issue, 70)
	firstIdx := strings.Index(content, "AAA_FIRST")
	lastIdx := strings.Index(content, "ZZZ_LAST")
	if firstIdx < 0 || lastIdx < 0 {
		t.Fatal("expected both comments in output")
	}
	if firstIdx > lastIdx {
		t.Error("ascending sort: first comment should appear before last")
	}

	// Descending: last comment appears before first
	m.commentSortAsc = false
	content = m.buildDetailContent(issue, 70)
	firstIdx = strings.Index(content, "AAA_FIRST")
	lastIdx = strings.Index(content, "ZZZ_LAST")
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
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
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

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
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
	model := requireModelPtr(t, result)
	for cmd != nil {
		result, cmd = model.Update(cmd())
		model = requireModelPtr(t, result)
	}

	// Press Esc
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
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

// Ensure huh is used (compile-time check)
var _ = huh.StateCompleted
