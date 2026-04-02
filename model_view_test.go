package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// NewModel does not set m.demo; main.go sets it after construction. Demo fixtures must set m.demo = true.

const goldenDir = "testdata/golden"

// updateGoldens: run `go test ./... -args -update-goldens` to refresh View() snapshots.
// Do not snapshot views that include spinner animation (loading overlays); keep loading false.
var updateGoldens = flag.Bool("update-goldens", false, "update View() golden files under testdata/golden")

func TestMain(m *testing.M) {
	_ = os.Setenv("TERM", "xterm-256color")
	code := m.Run()
	os.Exit(code)
}

func viewContent(m Model) string {
	return m.View().Content
}

func normalizeView(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = ansi.Strip(s)
	return strings.TrimSpace(s) + "\n"
}

func goldenPath(name string) string {
	return filepath.Join(goldenDir, name+".txt")
}

func diffLinePrefix(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	max := len(aLines)
	if len(bLines) > max {
		max = len(bLines)
	}
	var sb strings.Builder
	for i := 0; i < max; i++ {
		var al, bl string
		if i < len(aLines) {
			al = aLines[i]
		}
		if i < len(bLines) {
			bl = bLines[i]
		}
		if al != bl {
			sb.WriteString(strings.Repeat("-", 40))
			sb.WriteString("\n")
			sb.WriteString("want: ")
			sb.WriteString(al)
			sb.WriteString("\n")
			sb.WriteString("got:  ")
			sb.WriteString(bl)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	got = normalizeView(got)

	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run: go test ./... -args -update-goldens)", path, err)
	}
	want := normalizeView(string(wantBytes))
	if got != want {
		t.Fatalf("golden %s mismatch:\n%s", name, diffLinePrefix(want, got))
	}
}

const maxDrainIters = 100

// drainCmds runs cmd and subsequent cmds until nil or maxDrainIters (fails on excess).
func drainCmds(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	mp := m
	for i := 0; i < maxDrainIters && cmd != nil; i++ {
		result, next := mp.Update(cmd())
		mp = requireModelPtr(t, result)
		cmd = next
	}
	if cmd != nil {
		t.Fatalf("drainCmds: still have cmd after %d iterations", maxDrainIters)
	}
	return mp
}

// applyMsgs applies each message, draining any command chain after each.
func applyMsgs(t *testing.T, m *Model, msgs ...tea.Msg) *Model {
	t.Helper()
	mp := m
	for _, msg := range msgs {
		result, cmd := mp.Update(msg)
		mp = requireModelPtr(t, result)
		mp = drainCmds(t, mp, cmd)
	}
	return mp
}

func withWindowSize(t *testing.T, m *Model, w, h int) *Model {
	t.Helper()
	return applyMsgs(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

func normalizeForAssertions(s string) string {
	return normalizeView(s)
}

func fixtureModelConfigured(t *testing.T) *Model {
	t.Helper()
	cfg := Config{
		LinearAPIKey:  "lin_api_test",
		TeamID:        "team-1",
		TeamKey:       "TST",
		Teams:         []TeamEntry{{ID: "team-1", Key: "TST"}},
		ClaudeCommand: "claude",
		WorktreeBase:  "../worktrees",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
	m := NewModel(cfg)
	m.useCmux = false
	return withWindowSize(t, &m, 100, 40)
}

func fixtureModelFirstRunSettings(t *testing.T) *Model {
	t.Helper()
	m := NewModel(DefaultConfig())
	m.useCmux = false
	mp := withWindowSize(t, &m, 100, 40)
	cmd := mp.buildSettingsForm()
	return drainCmds(t, mp, cmd)
}

func fixtureModelDemoList(t *testing.T) *Model {
	t.Helper()
	m := NewModel(DemoConfig())
	m.demo = true
	m.useCmux = false
	mp := withWindowSize(t, &m, 100, 40)
	issues := DemoIssues()
	if len(issues) > 2 {
		issues = issues[:2]
	}
	mp = applyMsgs(t, mp, issuesLoadedMsg{issues: issues})
	mp.viewer = DemoViewer()
	mp.updateListTitle()
	return mp
}

func fixtureModelEmptyAssigned(t *testing.T) *Model {
	t.Helper()
	mp := fixtureModelConfigured(t)
	mp.filter = FilterAssigned
	mp = applyMsgs(t, mp, issuesLoadedMsg{issues: nil})
	return mp
}

func TestGoldenViewSettingsFirstRun(t *testing.T) {
	m := fixtureModelFirstRunSettings(t)
	assertGolden(t, "settings_first_run", viewContent(*m))
}

func TestGoldenViewListDemoIssues(t *testing.T) {
	m := fixtureModelDemoList(t)
	assertGolden(t, "list_demo_issues", viewContent(*m))
}

func TestGoldenViewListEmptyAssigned(t *testing.T) {
	m := fixtureModelEmptyAssigned(t)
	assertGolden(t, "list_empty_assigned", viewContent(*m))
}

func TestSequenceListEnterDetailBack(t *testing.T) {
	m := fixtureModelDemoList(t)
	// First issue is demo-1 (comments path in demo mode).
	m = applyMsgs(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.view != viewDetail {
		t.Fatalf("view = %v, want viewDetail", m.view)
	}
	if m.detailIssue == nil || m.detailIssue.ID != "demo-1" {
		t.Fatalf("detailIssue = %v, want demo-1", m.detailIssue)
	}
	out := normalizeForAssertions(viewContent(*m))
	if !strings.Contains(out, "ENG-142") {
		lim := min(400, len(out))
		t.Fatalf("detail view should mention issue id; got excerpt:\n%s", out[:lim])
	}

	m = applyMsgs(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.view != viewList {
		t.Fatalf("view = %v, want viewList after back", m.view)
	}
	if m.detailIssue != nil {
		t.Fatal("expected detailIssue cleared")
	}
}

func TestSequenceHelpToggle(t *testing.T) {
	m := fixtureModelDemoList(t)
	m = applyMsgs(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.showHelp {
		t.Fatal("expected showHelp true after ?")
	}
	out := normalizeForAssertions(viewContent(*m))
	if !strings.Contains(out, "Keybindings") {
		lim := min(500, len(out))
		t.Fatalf("help overlay should contain Keybindings; got:\n%s", out[:lim])
	}
	m = applyMsgs(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.showHelp {
		t.Fatal("expected showHelp false after second ?")
	}
}

func TestSequenceSearchModeEsc(t *testing.T) {
	m := fixtureModelDemoList(t)
	m = applyMsgs(t, m, tea.KeyPressMsg{Code: 'S', Text: "S"})
	if m.view != viewSearch {
		t.Fatalf("view = %v, want viewSearch", m.view)
	}
	m = applyMsgs(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.view != viewList {
		t.Fatalf("view = %v, want viewList after esc", m.view)
	}
}

func TestSequenceFilterCycleTab(t *testing.T) {
	m := fixtureModelDemoList(t)
	prev := m.filter
	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = requireModelPtr(t, result)
	if m.filter == prev {
		t.Fatal("expected filter to change after tab")
	}
	if !m.loading {
		t.Fatal("expected loading after filter cycle (fetch started)")
	}
	if cmd == nil {
		t.Fatal("expected fetch cmd after tab (do not drain: would call Linear API)")
	}
	// Simulate fetch completion without running network command.
	m.loading = false
	m = applyMsgs(t, m, issuesLoadedMsg{issues: DemoIssues()[:2]})
	if m.view != viewList {
		t.Fatalf("view = %v, want viewList", m.view)
	}
}
