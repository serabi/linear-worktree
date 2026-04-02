package main

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	vp := viewport.New(viewport.WithWidth(38), viewport.WithHeight(20))

	launchDelegate := list.NewDefaultDelegate()
	launchDelegate.SetHeight(2)
	launchDelegate.SetSpacing(0)
	ll := list.New([]list.Item{}, launchDelegate, 0, 0)
	ll.SetShowHelp(false)
	ll.SetShowStatusBar(false)
	ll.SetFilteringEnabled(false)
	ll.Styles.Title = titleStyle

	linkDelegate := list.NewDefaultDelegate()
	linkDelegate.ShowDescription = false
	linkDelegate.SetHeight(1)
	linkDelegate.SetSpacing(0)
	lnk := list.New([]list.Item{}, linkDelegate, 0, 0)
	lnk.SetShowHelp(false)
	lnk.SetShowStatusBar(false)
	lnk.SetFilteringEnabled(false)
	lnk.Styles.Title = titleStyle

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
		linkList:         lnk,
		promptArea:       ta,
		teamCache:        make(map[string]*teamState),
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
