package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

type viewMode int

const (
	viewList viewMode = iota
	viewSettings
	viewComment
	viewDetail
	viewLaunch
	viewPrompt
	viewProjectPicker
	viewStatePicker
	viewFilterPicker
	viewSearch
	viewLinkList
	viewSortPicker
	viewLabelPicker
	viewWorktreeList
)

type settingsDraft struct {
	apiKey     string
	teamKey    string
	wtBase     string
	copyFiles  string
	copyDirs   string
	claudeCmd  string
	claudeArgs string
	branch     string
	maxSlots   int
	hook       string
	prompt     string
	slotColors [absoluteMaxSlots]string
}

type teamState struct {
	issues         []Issue
	projects       []Project
	labels         []IssueLabel
	workflowStates []WorkflowState
	filter         FilterMode
	projectFilter  *string
	projectName    string
	labelFilter    *string
	labelName      string
	listIndex      int
	customViews    []CustomView
	activeViewIdx  int
}

type Model struct {
	cfg              Config
	list             list.Model
	issues           []Issue
	worktreeBranches map[string]bool
	worktreePaths    map[string]string
	filter           FilterMode
	sortMode         SortMode
	view             viewMode
	statusMsg        string
	detailIssue      *Issue
	width            int
	height           int

	teamCache map[string]*teamState

	cmuxClient  *CmuxClient
	paneManager *PaneManager
	useCmux     bool

	commentInput textinput.Model
	commentIssue *Issue

	cachedComments  []Comment
	cachedCommentID string
	commentSortAsc  bool

	detailViewport viewport.Model

	help         help.Model
	showHelp     bool
	keys         keyMap
	spinner      spinner.Model
	loading      bool
	loadingLabel string

	launchIssue *Issue
	launchList  list.Model
	promptArea  textarea.Model

	settingsTabs      [4]*huh.Form
	settingsTabNames  [4]string
	settingsActiveTab int
	settingsDraft     *settingsDraft
	settingsFirstRun  bool

	viewer *Viewer

	projects      []Project
	projectFilter *string
	projectName   string
	projectForm   *huh.Form

	labels      []IssueLabel
	labelFilter *string
	labelName   string
	labelForm   *huh.Form

	workflowStates []WorkflowState
	stateForm      *huh.Form
	stateIssue     *Issue

	customViews   []CustomView
	activeViewIdx int // 0 = "All Issues", 1+ = custom view index

	filterForm *huh.Form
	sortForm   *huh.Form

	searchInput textinput.Model
	searching   bool
	searchTerm  string
	savedIssues []Issue

	linkList         list.Model
	linkReturnToView viewMode

	worktreeList list.Model

	detailHistory       []*Issue
	pendingHistoryIssue *Issue

	prefetchSeq   int
	lastListIndex int

	confirm *confirmDialog

	// Demo mode
	demo bool
}

type confirmAction int

const (
	confirmQuit confirmAction = iota
	confirmPostComment
	confirmAssign
	confirmUnassign
	confirmStateChange
	confirmRemoveWorktree
)

type confirmDialog struct {
	action  confirmAction
	title   string
	message string
	onYes   func(m *Model) (tea.Model, tea.Cmd)
}

type keyMap struct {
	Navigate        key.Binding
	Claude          key.Binding
	Worktree     key.Binding
	Remove       key.Binding
	WorktreeList key.Binding
	Comment    key.Binding
	Detail     key.Binding
	Filter     key.Binding
	FilterPick key.Binding
	Open       key.Binding
	Refresh    key.Binding
	Search     key.Binding
	Setup      key.Binding
	Project    key.Binding
	Assign     key.Binding
	Unassign   key.Binding
	Label      key.Binding
	Links      key.Binding
	TeamSwitch key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeyMap(multiTeam bool) keyMap {
	km := keyMap{
		Navigate:   key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "navigate")),
		Claude:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "claude+worktree")),
		Worktree:     key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "create worktree")),
		Remove:       key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "remove worktree")),
		WorktreeList: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "worktrees")),
		Comment:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "comment")),
		Detail:     key.NewBinding(key.WithKeys("d", "enter"), key.WithHelp("enter/d", "detail")),
		Filter:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle filter")),
		FilterPick: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter picker")),
		Open:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "open")),
		Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Setup:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "settings")),
		Project:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign to me")),
		Unassign:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unassign")),
		Label:      key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "labels")),
		Links:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "links")),
		TeamSwitch: key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "switch team"), key.WithDisabled()),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
	if multiTeam {
		km.TeamSwitch.SetEnabled(true)
	}
	return km
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Claude, k.Detail, k.Project, k.Label, k.Filter, k.FilterPick, k.Setup, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Navigate, k.Claude, k.Worktree, k.Remove, k.WorktreeList},
		{k.Detail, k.Filter, k.FilterPick, k.Search},
		{k.Project, k.Label, k.Assign, k.Unassign, k.Links, k.TeamSwitch},
		{k.Open, k.Refresh, k.Setup, k.Help, k.Quit},
	}
}

func (m *Model) getBranchName(identifier string) string {
	return m.cfg.BranchPrefix + strings.ToLower(identifier)
}

func (m *Model) hasWorktree(identifier string) bool {
	return m.worktreeBranches[m.getBranchName(identifier)]
}

func (m *Model) worktreePathFor(identifier string) string {
	return m.worktreePaths[m.getBranchName(identifier)]
}

func (m *Model) selectedIssue() *Issue {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	if ii, ok := item.(issueItem); ok {
		return &ii.issue
	}
	return nil
}

func (m *Model) rebuildList() {
	items := make([]list.Item, len(m.issues))
	for i, issue := range m.issues {
		item := issueItem{
			issue:       issue,
			hasWorktree: m.hasWorktree(issue.Identifier),
			slotIdx:     -1,
		}
		if m.paneManager != nil {
			if slot, _ := m.paneManager.FindSlotByIdentifier(issue.Identifier); slot != nil {
				item.slotIdx = slot.Index
				item.slotStatus = slot.Status
				item.slotColor = m.cfg.SlotColorName(slot.Index)
			}
		}
		items[i] = item
	}
	m.list.SetItems(items)
}

func (m Model) activeViewName() string {
	if m.activeViewIdx > 0 && m.activeViewIdx-1 < len(m.customViews) {
		return m.customViews[m.activeViewIdx-1].Name
	}
	return ""
}

func (m Model) buildStatusLine() string {
	parts := []string{}
	scope := m.cfg.TeamKey
	if name := m.activeViewName(); name != "" {
		scope += " > " + name
	} else {
		if m.projectName != "" {
			scope += " > " + m.projectName
		}
		if m.labelName != "" {
			scope += " > label:" + m.labelName
		}
	}
	parts = append(parts, scope)
	parts = append(parts, fmt.Sprintf("%d issues", len(m.issues)))
	if m.activeViewIdx == 0 {
		parts = append(parts, m.filter.String())
		parts = append(parts, m.sortMode.String())
	}
	if m.useCmux && m.paneManager != nil {
		parts = append(parts, fmt.Sprintf("slots: %d/%d", m.paneManager.ActiveCount(), m.cfg.MaxSlots))
	}
	return strings.Join(parts, " | ")
}

func (m *Model) updateListTitle() {
	var parts []string
	if len(m.cfg.Teams) <= 1 {
		parts = append(parts, m.cfg.TeamKey)
	}
	if name := m.activeViewName(); name != "" {
		parts = append(parts, "["+name+"]")
	} else {
		if m.projectName != "" {
			parts = append(parts, m.projectName)
		}
		if m.labelName != "" {
			parts = append(parts, "label:"+m.labelName)
		}
		parts = append(parts, "["+m.filter.String()+"]")
	}
	m.list.Title = strings.Join(parts, " > ")
	if m.list.Title == "" {
		m.list.Title = "Issues"
	}
}

func (m *Model) saveTeamState() {
	if m.cfg.TeamKey == "" || m.issues == nil {
		return
	}
	issues := make([]Issue, len(m.issues))
	copy(issues, m.issues)
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	labels := make([]IssueLabel, len(m.labels))
	copy(labels, m.labels)
	states := make([]WorkflowState, len(m.workflowStates))
	copy(states, m.workflowStates)
	views := make([]CustomView, len(m.customViews))
	copy(views, m.customViews)
	m.teamCache[m.cfg.TeamKey] = &teamState{
		issues:         issues,
		projects:       projects,
		labels:         labels,
		workflowStates: states,
		filter:         m.filter,
		projectFilter:  m.projectFilter,
		projectName:    m.projectName,
		labelFilter:    m.labelFilter,
		labelName:      m.labelName,
		listIndex:      m.list.Index(),
		customViews:    views,
		activeViewIdx:  m.activeViewIdx,
	}
}

func (m *Model) restoreTeamState() bool {
	ts, ok := m.teamCache[m.cfg.TeamKey]
	if !ok {
		return false
	}
	m.issues = ts.issues
	m.projects = ts.projects
	m.labels = ts.labels
	m.workflowStates = ts.workflowStates
	m.filter = ts.filter
	m.projectFilter = ts.projectFilter
	m.projectName = ts.projectName
	m.labelFilter = ts.labelFilter
	m.labelName = ts.labelName
	m.customViews = ts.customViews
	m.activeViewIdx = ts.activeViewIdx
	m.rebuildList()
	m.list.Select(ts.listIndex)
	m.updateListTitle()
	return true
}

func (m *Model) flushTeamState() {
	m.issues = nil
	m.projects = nil
	m.labels = nil
	m.workflowStates = nil
	m.cachedComments = nil
	m.cachedCommentID = ""
	m.projectFilter = nil
	m.projectName = ""
	m.labelFilter = nil
	m.labelName = ""
	m.detailIssue = nil
	m.savedIssues = nil
	m.searchTerm = ""
	m.searching = false
	m.stateIssue = nil
	m.stateForm = nil
	m.filter = FilterAssigned
	m.activeViewIdx = 0
	m.customViews = nil
	m.view = viewList
	m.list.SetItems(nil)
	m.updateListTitle()
}
