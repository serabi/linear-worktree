package main

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func NewModel(cfg Config) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.SetSpacing(0)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = cfg.TeamKey
	if l.Title == "" {
		l.Title = "linear-worktree"
	}
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	commentIn := textinput.New()
	commentIn.Placeholder = "Type your comment..."
	commentIn.CharLimit = 2000

	searchIn := textinput.New()
	searchIn.Placeholder = "Search issues..."
	searchIn.CharLimit = 200

	cmuxClient := NewCmuxClient()
	useCmux := cmuxClient.Available()

	var pm *PaneManager
	if useCmux {
		pm = NewPaneManager(cmuxClient, cfg.MaxSlots)
		pm.RenameWorkspace("linear-worktree")
		pm.renameTab(pm.tuiSurface, "linear-worktree")
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	h := help.New()
	h.ShortSeparator = " · "
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(dimColor)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(dimColor)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(faintColor)
	h.Styles.FullSeparator = h.Styles.FullSeparator.Foreground(faintColor)

	vp := viewport.New(38, 20)

	launchDelegate := list.NewDefaultDelegate()
	launchDelegate.SetHeight(2)
	launchDelegate.SetSpacing(0)
	ll := list.New([]list.Item{}, launchDelegate, 0, 0)
	ll.SetShowHelp(false)
	ll.SetShowStatusBar(false)
	ll.SetFilteringEnabled(false)
	ll.Styles.Title = titleStyle

	ta := textarea.New()
	ta.Placeholder = "Enter your prompt for Claude..."
	ta.CharLimit = 10000

	m := Model{
		cfg:              cfg,
		list:             l,
		worktreeBranches: make(map[string]bool),
		filter:           FilterAssigned,
		view:             viewList,
		commentInput:     commentIn,
		searchInput:      searchIn,
		cmuxClient:       cmuxClient,
		paneManager:      pm,
		useCmux:          useCmux,
		help:             h,
		keys:             defaultKeyMap(len(cfg.Teams) > 1),
		spinner:          sp,
		detailViewport:   vp,
		launchList:       ll,
		promptArea:       ta,
	}
	if cfg.NeedsSetup() {
		m.settingsFirstRun = true
		m.initSettingsForm()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.settingsTabs[0] != nil {
		cmds := make([]tea.Cmd, len(m.settingsTabs))
		for i := range m.settingsTabs {
			cmds[i] = m.settingsTabs[i].Init()
		}
		return tea.Batch(cmds...)
	}
	if m.demo {
		return func() tea.Msg {
			return issuesLoadedMsg{issues: DemoIssues()}
		}
	}
	cmds := []tea.Cmd{
		m.fetchIssues(),
		m.fetchWorktrees(),
		m.fetchViewer(),
		m.fetchProjects(),
		m.detectBranchIssue(),
	}
	if m.useCmux {
		cmds = append(cmds, m.startStatusPoll())
	}
	return tea.Batch(cmds...)
}
