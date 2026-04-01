package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Adaptive colors for light/dark terminal themes.
	// AdaptiveColor: {Light, Dark}
	// Adaptive colors for WCAG AA contrast on both light and dark backgrounds.
	// Light values target >= 4.5:1 against #F5F5F5; dark values target visibility on #1E1E1E.
	dimColor       = lipgloss.AdaptiveColor{Light: "#444", Dark: "#888"}
	subtleColor    = lipgloss.AdaptiveColor{Light: "#555", Dark: "#555"}
	mutedColor     = lipgloss.AdaptiveColor{Light: "#555", Dark: "#666"}
	faintColor     = lipgloss.AdaptiveColor{Light: "#646464", Dark: "#444"}
	yellowColor    = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#EAB308"}
	identCyanColor = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#06B6D4"}
	greenColor     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#22C55E"}
	redColor       = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#EF4444"}
	orangeColor    = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#F97316"}
	blueColor      = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#3B82F6"}

	appStyle = lipgloss.NewStyle().Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Padding(0, 1)

	issueIdentStyle = lipgloss.NewStyle().
			Foreground(identCyanColor).
			Bold(true)

	worktreeMarker = lipgloss.NewStyle().
			Foreground(greenColor)

	urgentStyle = lipgloss.NewStyle().Foreground(redColor)
	highStyle   = lipgloss.NewStyle().Foreground(orangeColor)
	mediumStyle = lipgloss.NewStyle().Foreground(yellowColor)
	lowStyle    = lipgloss.NewStyle().Foreground(blueColor)

	setupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Width(50)

	slotRunningStyle = lipgloss.NewStyle().Foreground(greenColor)
	slotWaitingStyle = lipgloss.NewStyle().Foreground(yellowColor)
	slotIdleStyle    = lipgloss.NewStyle().Foreground(dimColor)
	slotEmptyStyle   = lipgloss.NewStyle().Foreground(faintColor)

	commentDimStyle = lipgloss.NewStyle().Foreground(dimColor)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(dimColor).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(faintColor).
				Padding(0, 2)
)

type issueItem struct {
	issue       Issue
	hasWorktree bool
	slotIdx     int
	slotStatus  AgentStatus
}

func (i issueItem) Title() string {
	icon := statusIcon(i.issue.State.Type)
	pri := priorityIcon(i.issue.Priority)
	wt := ""
	if i.hasWorktree {
		wt = worktreeMarker.Render(" 🌳")
	}

	slot := ""
	if i.slotIdx >= 0 {
		var style lipgloss.Style
		switch i.slotStatus {
		case AgentRunning:
			style = slotRunningStyle
		case AgentWaiting:
			style = slotWaitingStyle
		case AgentIdle:
			style = slotIdleStyle
		default:
			style = slotEmptyStyle
		}
		slot = style.Render(fmt.Sprintf(" [%d:%s]", i.slotIdx+1, i.slotStatus.String()))
	}

	return fmt.Sprintf("%s %s %s %s%s%s",
		icon, pri,
		issueIdentStyle.Render(i.issue.Identifier),
		i.issue.Title, wt, slot,
	)
}

func (i issueItem) Description() string {
	var parts []string
	if i.issue.Assignee != nil {
		name := i.issue.Assignee.DisplayName
		if name == "" {
			name = i.issue.Assignee.Name
		}
		if idx := strings.IndexByte(name, ' '); idx > 0 {
			name = name[:idx]
		}
		parts = append(parts, name)
	} else {
		parts = append(parts, commentDimStyle.Render("unassigned"))
	}

	if i.issue.Project != nil {
		parts = append(parts, i.issue.Project.Name)
	}

	if i.issue.DueDate != nil {
		if t, err := time.Parse("2006-01-02", *i.issue.DueDate); err == nil {
			days := int(time.Until(t).Hours() / 24)
			switch {
			case days < 0:
				parts = append(parts, fmt.Sprintf("OVERDUE %dd", -days))
			case days <= 3:
				parts = append(parts, fmt.Sprintf("Due in %dd", days))
			}
		}
	}

	if n := len(i.issue.Children.Nodes); n > 0 {
		done := 0
		for _, c := range i.issue.Children.Nodes {
			if c.State.Type == "completed" {
				done++
			}
		}
		parts = append(parts, fmt.Sprintf("[%d/%d]", done, n))
	}

	if labels := i.issue.Labels.Nodes; len(labels) > 0 {
		maxLabels := 2
		if len(labels) < maxLabels {
			maxLabels = len(labels)
		}
		for _, l := range labels[:maxLabels] {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color))
			parts = append(parts, style.Render(l.Name))
		}
		if remaining := len(labels) - maxLabels; remaining > 0 {
			parts = append(parts, commentDimStyle.Render(fmt.Sprintf("+%d", remaining)))
		}
	}

	return strings.Join(parts, " | ")
}

func (i issueItem) FilterValue() string {
	return i.issue.Identifier + " " + i.issue.Title
}

type launchOption struct {
	action    string
	title     string
	desc      string
	slotIndex int
}

func (l launchOption) Title() string       { return l.title }
func (l launchOption) Description() string { return l.desc }
func (l launchOption) FilterValue() string { return l.title }

func statusIcon(stateType string) string {
	switch stateType {
	case "backlog":
		return lipgloss.NewStyle().Foreground(subtleColor).Render("○")
	case "unstarted":
		return lipgloss.NewStyle().Foreground(dimColor).Render("○")
	case "started":
		return lipgloss.NewStyle().Foreground(yellowColor).Render("●")
	case "completed":
		return lipgloss.NewStyle().Foreground(greenColor).Render("✓")
	case "cancelled":
		return lipgloss.NewStyle().Foreground(mutedColor).Render("✗")
	default:
		return "?"
	}
}

func priorityIcon(p int) string {
	switch p {
	case 1:
		return urgentStyle.Render("▲")
	case 2:
		return highStyle.Render("▲")
	case 3:
		return mediumStyle.Render("■")
	case 4:
		return lowStyle.Render("▼")
	default:
		return " "
	}
}

type issuesLoadedMsg struct {
	issues []Issue
	err    error
}

type worktreesLoadedMsg struct {
	branches map[string]bool
}

type worktreeCreatedMsg struct {
	path       string
	identifier string
	err        error
	hookErr    error
}

type claudeLaunchedMsg struct {
	identifier string
	err        error
}

type cmuxSlotOpenedMsg struct {
	slotIdx    int
	identifier string
	wtPath     string
	err        error
}

type teamsLoadedMsg struct {
	err error
}

type setupCompleteMsg struct {
	cfg Config
}

type commentPostedMsg struct {
	identifier string
	err        error
}

type commentsLoadedMsg struct {
	issueID  string
	comments []Comment
	err      error
}

type launchReadyMsg struct {
	issue  Issue
	wtPath string
	prompt string
}

type statusPollMsg struct{}

type viewerLoadedMsg struct {
	viewer *Viewer
	err    error
}

type projectsLoadedMsg struct {
	projects []Project
	err      error
}

type statesLoadedMsg struct {
	states []WorkflowState
	err    error
}

type issueAssignedMsg struct {
	identifier string
	err        error
}

type issueUnassignedMsg struct {
	identifier string
	err        error
}

type issueStateChangedMsg struct {
	identifier string
	err        error
}

type branchIssueFoundMsg struct {
	issue *Issue
}

type searchResultsMsg struct {
	issues []Issue
	err    error
}

type prefetchTickMsg struct {
	seq int
}

type teamSwitchedMsg struct {
	cfg Config
	err error
}

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
	viewLinkPicker
	viewSortPicker
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
}

type teamState struct {
	issues         []Issue
	projects       []Project
	workflowStates []WorkflowState
	filter         FilterMode
	projectFilter  *string
	projectName    string
	listIndex      int
}

type Model struct {
	cfg              Config
	list             list.Model
	issues           []Issue
	worktreeBranches map[string]bool
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

	settingsTabs      [3]*huh.Form
	settingsTabNames  [3]string
	settingsActiveTab int
	settingsDraft     *settingsDraft
	settingsFirstRun  bool

	viewer *Viewer

	projects      []Project
	projectFilter *string
	projectName   string
	projectForm   *huh.Form

	workflowStates []WorkflowState
	stateForm      *huh.Form
	stateIssue     *Issue

	filterForm *huh.Form
	sortForm   *huh.Form

	searchInput textinput.Model
	searching   bool
	searchTerm  string
	savedIssues []Issue

	linkPickerForm *huh.Form
	linkPickerURLs []string
	linkSelected   string

	prefetchSeq   int
	lastListIndex int

	confirm *confirmDialog

	// Demo mode
	demo bool
}

type confirmAction int

const (
	confirmQuit confirmAction = iota
	confirmCloseSlot
	confirmPostComment
	confirmAssign
	confirmUnassign
	confirmStateChange
)

type confirmDialog struct {
	action  confirmAction
	title   string
	message string
	onYes   func(m *Model) (tea.Model, tea.Cmd)
}

type keyMap struct {
	Navigate   key.Binding
	Claude     key.Binding
	Worktree   key.Binding
	Close      key.Binding
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
	Links      key.Binding
	TeamSwitch key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeyMap(multiTeam bool) keyMap {
	km := keyMap{
		Navigate:   key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "navigate")),
		Claude:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "claude+worktree")),
		Worktree:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "worktree")),
		Close:      key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close slot")),
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
	return []key.Binding{k.Claude, k.Detail, k.Project, k.Filter, k.FilterPick, k.Setup, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Navigate, k.Claude, k.Worktree, k.Close},
		{k.Detail, k.Filter, k.FilterPick, k.Search},
		{k.Project, k.Assign, k.Unassign, k.Links, k.TeamSwitch},
		{k.Open, k.Refresh, k.Setup, k.Help, k.Quit},
	}
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
	slotMap := make(map[string]*WorktreeSlot)
	if m.paneManager != nil {
		for _, slot := range m.paneManager.Slots() {
			if slot != nil {
				slotMap[slot.Issue.Identifier] = slot
			}
		}
	}

	items := make([]list.Item, len(m.issues))
	for i, issue := range m.issues {
		branch := m.cfg.BranchPrefix + strings.ToLower(issue.Identifier)
		item := issueItem{
			issue:       issue,
			hasWorktree: m.worktreeBranches[branch],
			slotIdx:     -1,
		}
		if slot, ok := slotMap[issue.Identifier]; ok {
			item.slotIdx = slot.Index
			item.slotStatus = slot.Status
		}
		items[i] = item
	}
	m.list.SetItems(items)
}

func (m Model) buildStatusLine() string {
	parts := []string{}
	if m.projectName != "" {
		parts = append(parts, m.cfg.TeamKey+" > "+m.projectName)
	} else {
		parts = append(parts, m.cfg.TeamKey)
	}
	parts = append(parts, fmt.Sprintf("%d issues", len(m.issues)))
	parts = append(parts, m.filter.String())
	parts = append(parts, m.sortMode.String())
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
	if m.projectName != "" {
		parts = append(parts, m.projectName)
	}
	parts = append(parts, "["+m.filter.String()+"]")
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
	states := make([]WorkflowState, len(m.workflowStates))
	copy(states, m.workflowStates)
	m.teamCache[m.cfg.TeamKey] = &teamState{
		issues:         issues,
		projects:       projects,
		workflowStates: states,
		filter:         m.filter,
		projectFilter:  m.projectFilter,
		projectName:    m.projectName,
		listIndex:      m.list.Index(),
	}
}

func (m *Model) restoreTeamState() bool {
	ts, ok := m.teamCache[m.cfg.TeamKey]
	if !ok {
		return false
	}
	m.issues = ts.issues
	m.projects = ts.projects
	m.workflowStates = ts.workflowStates
	m.filter = ts.filter
	m.projectFilter = ts.projectFilter
	m.projectName = ts.projectName
	m.rebuildList()
	m.list.Select(ts.listIndex)
	m.updateListTitle()
	return true
}

func (m *Model) flushTeamState() {
	m.issues = nil
	m.projects = nil
	m.workflowStates = nil
	m.cachedComments = nil
	m.cachedCommentID = ""
	m.projectFilter = nil
	m.projectName = ""
	m.detailIssue = nil
	m.savedIssues = nil
	m.searchTerm = ""
	m.searching = false
	m.stateIssue = nil
	m.stateForm = nil
	m.filter = FilterAssigned
	m.view = viewList
	m.list.SetItems(nil)
	m.updateListTitle()
}
