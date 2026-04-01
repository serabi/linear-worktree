package main

import (
	"fmt"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// --- Styles ---

var (
	appStyle = lipgloss.NewStyle().Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Padding(0, 1)

	issueIdentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06B6D4")).
			Bold(true)

	worktreeMarker = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22C55E"))

	urgentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	highStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316"))
	mediumStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308"))
	lowStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))

	setupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Width(50)

	slotRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	slotWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308"))
	slotIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	slotEmptyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#444"))

	commentDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
)

// --- Issue list item ---

type issueItem struct {
	issue       Issue
	hasWorktree bool
	slotIdx     int // -1 if not in a slot
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
		parts = append(parts, name)
	} else {
		parts = append(parts, "Unassigned")
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
	return strings.Join(parts, " | ")
}
func (i issueItem) FilterValue() string {
	return i.issue.Identifier + " " + i.issue.Title
}

// launchOption represents a menu item in the launch picker.
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
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#555")).Render("○")
	case "unstarted":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render("○")
	case "started":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")).Render("●")
	case "completed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Render("✓")
	case "cancelled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#666")).Render("✗")
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

// --- Messages ---

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

// --- App state ---

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
)


// --- Model ---

type Model struct {
	cfg              Config
	list             list.Model
	issues           []Issue
	worktreeBranches map[string]bool
	filter           FilterMode
	view             viewMode
	statusMsg        string
	detailIssue      *Issue
	width            int
	height           int

	// Cmux E-layout
	cmuxClient  *CmuxClient
	paneManager *PaneManager
	useCmux     bool

	// Comment input
	commentInput textinput.Model
	commentIssue *Issue // which issue we're commenting on

	// Comments cache for detail view
	cachedComments  []Comment
	cachedCommentID string

	// Detail viewport
	detailViewport viewport.Model

	// Help + spinner
	help    help.Model
	keys    keyMap
	spinner      spinner.Model
	loading      bool
	loadingLabel string

	// Launch menu + prompt editor
	launchIssue *Issue
	launchList  list.Model
	promptArea  textarea.Model

	// Settings tabs
	settingsTabs      [3]*huh.Form
	settingsTabNames  [3]string
	settingsActiveTab int
	settingsAPIKey    string
	settingsTeamKey   string
	settingsWtBase    string
	settingsCopyFiles string
	settingsCopyDirs  string
	settingsClaudeCmd string
	settingsClaudeArgs string
	settingsBranch    string
	settingsMaxSlots  int
	settingsHook      string
	settingsPrompt    string
	settingsFirstRun  bool

	// Viewer (authenticated user)
	viewer *Viewer

	// Project filtering
	projects      []Project
	projectFilter *string // nil = all, "none" = no project, else project ID
	projectName   string  // for status bar display
	projectForm   *huh.Form

	// State transition
	workflowStates []WorkflowState
	stateForm      *huh.Form
	stateIssue     *Issue

	// Filter picker
	filterForm *huh.Form

	// Server search
	searchInput textinput.Model
	searching   bool
	searchTerm  string
	savedIssues []Issue // stash regular issues while showing search results
}

// keyMap defines keybindings for the help component.
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
	State      key.Binding
	Assign     key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
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
		Project:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "project")),
		State:      key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
		Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign to me")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Claude, k.Detail, k.Project, k.FilterPick, k.Setup, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Navigate, k.Claude, k.Worktree, k.Close},
		{k.Comment, k.Detail, k.Filter, k.FilterPick, k.Search},
		{k.Project, k.State, k.Assign, k.Open},
		{k.Refresh, k.Setup, k.Help, k.Quit},
	}
}

func NewModel(cfg Config) Model {
	// Issue list
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

	// Comment input
	commentIn := textinput.New()
	commentIn.Placeholder = "Type your comment..."
	commentIn.CharLimit = 2000

	// Search input
	searchIn := textinput.New()
	searchIn.Placeholder = "Search issues..."
	searchIn.CharLimit = 200

	// Check for cmux
	cmuxClient := NewCmuxClient()
	useCmux := cmuxClient.Available()

	var pm *PaneManager
	if useCmux {
		pm = NewPaneManager(cmuxClient, cfg.MaxSlots)
		pm.RenameWorkspace("linear-worktree")
		pm.renameTab(pm.tuiSurface, "linear-worktree")
	}

	// Spinner for loading states
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	// Help component
	h := help.New()
	h.ShortSeparator = " · "

	// Detail viewport
	vp := viewport.New(38, 20)

	// Launch menu list
	launchDelegate := list.NewDefaultDelegate()
	launchDelegate.SetHeight(2)
	launchDelegate.SetSpacing(0)
	ll := list.New([]list.Item{}, launchDelegate, 0, 0)
	ll.SetShowHelp(false)
	ll.SetShowStatusBar(false)
	ll.SetFilteringEnabled(false)
	ll.Styles.Title = titleStyle

	// Prompt textarea
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
		keys:             defaultKeyMap(),
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

// --- Commands ---

func (m Model) fetchIssues() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		if m.projectFilter != nil {
			if *m.projectFilter == "none" {
				issues, _, err := client.GetIssuesWithNoProject(m.cfg.TeamID, m.filter, "")
				return issuesLoadedMsg{issues: issues, err: err}
			}
			issues, _, err := client.GetIssuesByProject(m.cfg.TeamID, *m.projectFilter, "")
			return issuesLoadedMsg{issues: issues, err: err}
		}
		issues, _, err := client.GetIssues(m.cfg.TeamID, m.filter, "")
		return issuesLoadedMsg{issues: issues, err: err}
	}
}

func (m Model) fetchWorktrees() tea.Cmd {
	return func() tea.Msg {
		wts, err := ListWorktrees()
		branches := make(map[string]bool)
		if err == nil {
			for _, wt := range wts {
				branches[wt.Branch] = true
			}
		}
		return worktreesLoadedMsg{branches: branches}
	}
}

func (m Model) launchClaudeCmd(wtPath string, issue Issue) tea.Cmd {
	return func() tea.Msg {
		err := LaunchClaude(wtPath, issue, m.cfg)
		return claudeLaunchedMsg{identifier: issue.Identifier, err: err}
	}
}

func (m Model) openCmuxSlotWithPromptCmd(issue Issue, wtPath, prompt string) tea.Cmd {
	return func() tea.Msg {
		slot, err := m.paneManager.OpenSlotWithPrompt(issue, wtPath, prompt, m.cfg)
		if err != nil {
			return cmuxSlotOpenedMsg{err: err, identifier: issue.Identifier, wtPath: wtPath}
		}
		return cmuxSlotOpenedMsg{slotIdx: slot.Index, identifier: issue.Identifier, wtPath: wtPath}
	}
}

func (m Model) launchClaudeWithPromptCmd(wtPath string, issue Issue, prompt string) tea.Cmd {
	return func() tea.Msg {
		err := LaunchClaudeWithPrompt(wtPath, issue, prompt, m.cfg)
		return claudeLaunchedMsg{identifier: issue.Identifier, err: err}
	}
}

func (m Model) postCommentCmd(issueID, body string, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.AddComment(issueID, body)
		return commentPostedMsg{identifier: identifier, err: err}
	}
}

func (m Model) fetchCommentsCmd(issueID string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		comments, err := client.GetComments(issueID)
		return commentsLoadedMsg{issueID: issueID, comments: comments, err: err}
	}
}

func (m Model) fetchViewer() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		viewer, err := client.GetViewer()
		return viewerLoadedMsg{viewer: viewer, err: err}
	}
}

func (m Model) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		projects, err := client.GetProjects(m.cfg.TeamID)
		return projectsLoadedMsg{projects: projects, err: err}
	}
}

func (m Model) fetchWorkflowStates() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		states, err := client.GetWorkflowStates(m.cfg.TeamID)
		return statesLoadedMsg{states: states, err: err}
	}
}

func (m Model) assignToMeCmd(issueID, assigneeID, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.UpdateIssueAssignee(issueID, assigneeID)
		return issueAssignedMsg{identifier: identifier, err: err}
	}
}

func (m Model) changeStateCmd(issueID, stateID, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.UpdateIssueState(issueID, stateID)
		return issueStateChangedMsg{identifier: identifier, err: err}
	}
}

func (m Model) detectBranchIssue() tea.Cmd {
	return func() tea.Msg {
		out, err := osexec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err != nil {
			return branchIssueFoundMsg{}
		}
		branch := strings.TrimSpace(string(out))
		if !strings.HasPrefix(branch, m.cfg.BranchPrefix) {
			return branchIssueFoundMsg{}
		}
		client := NewLinearClient(m.cfg.LinearAPIKey)
		issue, _ := client.SearchIssueByBranch(branch)
		return branchIssueFoundMsg{issue: issue}
	}
}

func (m Model) resolveTeamCmd(apiKey, teamKey string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(apiKey)
		team, err := client.GetTeamByKey(teamKey)
		if err != nil {
			return teamsLoadedMsg{err: err}
		}

		cfg := m.cfg
		cfg.LinearAPIKey = apiKey
		cfg.TeamID = team.ID
		cfg.TeamKey = team.Key
		if err := SaveConfig(cfg); err != nil {
			return teamsLoadedMsg{err: err}
		}
		return setupCompleteMsg{cfg: cfg}
	}
}

func (m Model) startStatusPoll() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return statusPollMsg{}
	})
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-2, msg.Height-4)
		if m.settingsTabs[0] != nil {
			w := msg.Width - 4
			if w < 60 {
				w = 60
			}
			for i := range m.settingsTabs {
				if m.settingsTabs[i] != nil {
					m.settingsTabs[i] = m.settingsTabs[i].WithWidth(w)
				}
			}
		}
		if m.view == viewDetail && m.detailIssue != nil {
			contentWidth := msg.Width - 6
			m.detailViewport.Width = contentWidth
			m.detailViewport.Height = msg.Height - 6
			m.detailViewport.SetContent(m.buildDetailContent(m.detailIssue, contentWidth))
		}
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys when list is filtering
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		switch m.view {
		case viewSettings:
			return m.updateSettings(msg)
		case viewComment:
			return m.updateComment(msg)
		case viewDetail:
			return m.updateDetail(msg)
		case viewLaunch:
			return m.updateLaunch(msg)
		case viewPrompt:
			return m.updatePrompt(msg)
		case viewProjectPicker:
			return m.updateProjectPicker(msg)
		case viewStatePicker:
			return m.updateStatePicker(msg)
		case viewFilterPicker:
			return m.updateFilterPicker(msg)
		case viewSearch:
			return m.updateSearch(msg)
		default:
			return m.updateList(msg)
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.statusMsg = m.spinner.View() + " " + m.loadingLabel
			return m, cmd
		}
		return m, nil

	case issuesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.issues = msg.issues
		m.rebuildList()
		m.updateListTitle()
		m.statusMsg = m.buildStatusLine()
		return m, nil

	case worktreesLoadedMsg:
		m.worktreeBranches = msg.branches
		m.rebuildList()
		return m, nil

	case worktreeCreatedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error creating worktree: %v", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Worktree created: %s", msg.path)
		return m, m.fetchWorktrees()

	case cmuxSlotOpenedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error opening slot: %v", msg.err)
			// Fall back to tmux
			for _, issue := range m.issues {
				if issue.Identifier == msg.identifier {
					return m, m.launchClaudeCmd(msg.wtPath, issue)
				}
			}
		} else {
			m.statusMsg = fmt.Sprintf("Slot %d: %s", msg.slotIdx+1, msg.identifier)
			m.rebuildList()
		}
		return m, nil

	case claudeLaunchedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error launching Claude: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Claude launched for %s", msg.identifier)
		}
		return m, nil

	case commentPostedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Comment error: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Comment posted on %s", msg.identifier)
			// Refresh comments in detail view
			if m.commentIssue != nil {
				return m, m.fetchCommentsCmd(m.commentIssue.ID)
			}
		}
		return m, nil

	case commentsLoadedMsg:
		if msg.err == nil {
			m.cachedComments = msg.comments
			m.cachedCommentID = msg.issueID
			if m.view == viewDetail && m.detailIssue != nil && m.detailIssue.ID == msg.issueID {
				contentWidth := m.width - 6
				m.detailViewport.SetContent(m.buildDetailContent(m.detailIssue, contentWidth))
			}
		}
		return m, nil

	case launchReadyMsg:
		if m.useCmux && m.paneManager != nil {
			return m, m.openCmuxSlotWithPromptCmd(msg.issue, msg.wtPath, msg.prompt)
		}
		return m, m.launchClaudeWithPromptCmd(msg.wtPath, msg.issue, msg.prompt)

	case statusPollMsg:
		if m.paneManager != nil {
			m.paneManager.PollStatus()
			m.rebuildList()
		}
		return m, m.startStatusPoll()

	case setupCompleteMsg:
		m.cfg = msg.cfg
		m.view = viewList
		m.settingsTabs = [3]*huh.Form{}
		m.settingsFirstRun = false
		m.statusMsg = "Settings saved. API key stored in OS keychain."
		m.updateListTitle()
		if m.paneManager != nil {
			m.paneManager = NewPaneManager(m.cmuxClient, m.cfg.MaxSlots)
		}
		cmds := []tea.Cmd{m.fetchIssues(), m.fetchWorktrees(), m.fetchViewer(), m.fetchProjects()}
		if m.useCmux {
			cmds = append(cmds, m.startStatusPoll())
		}
		return m, tea.Batch(cmds...)

	case teamsLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Setup error: %v", msg.err)
		}
		return m, nil

	case viewerLoadedMsg:
		if msg.err == nil {
			m.viewer = msg.viewer
		}
		return m, nil

	case projectsLoadedMsg:
		if msg.err == nil {
			m.projects = msg.projects
		}
		return m, nil

	case statesLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error loading states: %v", msg.err)
			return m, nil
		}
		m.workflowStates = msg.states
		return m, m.showStatePicker()

	case issueAssignedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Assign error: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Assigned %s to you", msg.identifier)
			return m, m.fetchIssues()
		}
		return m, nil

	case issueStateChangedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("State change error: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Updated state for %s", msg.identifier)
			return m, m.fetchIssues()
		}
		return m, nil

	case searchResultsMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Search error: %v", msg.err)
			return m, nil
		}
		m.searching = true
		m.issues = msg.issues
		m.rebuildList()
		m.statusMsg = fmt.Sprintf("Search: \"%s\" (%d results)", m.searchTerm, len(msg.issues))
		return m, nil

	case branchIssueFoundMsg:
		if msg.issue != nil {
			for i, item := range m.list.Items() {
				if ii, ok := item.(issueItem); ok && ii.issue.Identifier == msg.issue.Identifier {
					m.list.Select(i)
					m.statusMsg = fmt.Sprintf("Auto-selected %s (matches current branch)", msg.issue.Identifier)
					break
				}
			}
		}
		return m, nil
	}

	// Route non-key messages to active huh forms
	if m.view == viewSettings && m.settingsTabs[0] != nil {
		f := m.activeSettingsForm()
		form, cmd := f.Update(msg)
		if updated, ok := form.(*huh.Form); ok {
			m.settingsTabs[m.settingsActiveTab] = updated
		}
		active := m.activeSettingsForm()
		if active.State == huh.StateCompleted {
			if m.settingsActiveTab < len(m.settingsTabs)-1 {
				m.rebuildActiveTab()
				m.settingsActiveTab++
			} else {
				m.rebuildActiveTab()
			}
		} else if active.State == huh.StateAborted {
			m.rebuildActiveTab()
		}
		return m, cmd
	}
	if m.view == viewProjectPicker && m.projectForm != nil {
		form, cmd := m.projectForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.projectForm = f
		}
		if m.projectForm.State == huh.StateCompleted {
			return m.handleProjectSelected()
		}
		return m, cmd
	}
	if m.view == viewStatePicker && m.stateForm != nil {
		form, cmd := m.stateForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.stateForm = f
		}
		if m.stateForm.State == huh.StateCompleted {
			return m.handleStateSelected()
		}
		return m, cmd
	}
	if m.view == viewFilterPicker && m.filterForm != nil {
		form, cmd := m.filterForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.filterForm = f
		}
		if m.filterForm.State == huh.StateCompleted {
			return m.handleFilterSelected()
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
		return m, tea.Quit

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		m.filter = m.filter.Next()
		m.updateListTitle()
		m.loading = true
		m.loadingLabel = "Loading..."
		return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)

	case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
		return m, m.showFilterPicker()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		m.loading = true
		m.loadingLabel = "Refreshing..."
		return m, tea.Batch(m.fetchIssues(), m.fetchWorktrees(), m.spinner.Tick)

	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.launchIssue = issue
		m.view = viewLaunch

		// Build menu options dynamically
		items := []list.Item{
			launchOption{"prompt", "Launch with prompt", "Edit prompt before launching Claude", -1},
			launchOption{"blank", "Launch blank session", "Open Claude with no initial message", -1},
		}

		// Check for active slot
		if m.paneManager != nil {
			for _, slot := range m.paneManager.Slots() {
				if slot != nil && slot.Issue.Identifier == issue.Identifier {
					items = append([]list.Item{
						launchOption{"resume", "Resume existing session", fmt.Sprintf("Focus slot %d (%s)", slot.Index+1, slot.Status.Label()), slot.Index},
					}, items...)
					break
				}
			}
		}

		// Check for existing worktree without active slot
		branch := m.cfg.BranchPrefix + strings.ToLower(issue.Identifier)
		hasWorktree := m.worktreeBranches[branch]
		hasSlot := false
		if m.paneManager != nil {
			for _, slot := range m.paneManager.Slots() {
				if slot != nil && slot.Issue.Identifier == issue.Identifier {
					hasSlot = true
					break
				}
			}
		}
		if hasWorktree && !hasSlot {
			items = append(items, launchOption{"existing", "Open in existing worktree", "Launch Claude in the existing worktree", -1})
		}

		m.launchList.Title = fmt.Sprintf("Launch Claude for %s", issue.Identifier)
		m.launchList.SetItems(items)
		m.launchList.SetSize(m.width-4, len(items)*3+4)

		// Pre-fetch comments for prompt building
		var cmd tea.Cmd
		if issue.ID != m.cachedCommentID {
			cmd = m.fetchCommentsCmd(issue.ID)
		}
		return m, cmd

	case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Creating worktree for %s...", issue.Identifier)
		return m, func() tea.Msg {
			wtPath, err := CreateWorktree(issue.Identifier, m.cfg)
			if err != nil {
				return worktreeCreatedMsg{err: err, identifier: issue.Identifier}
			}
			RunPostCreateHook(wtPath, m.cfg)
			return worktreeCreatedMsg{path: wtPath, identifier: issue.Identifier}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		// Close a worktree slot
		if m.paneManager == nil {
			m.statusMsg = "No cmux panes to close"
			return m, nil
		}
		issue := m.selectedIssue()
		if issue == nil {
			return m, nil
		}
		// Find slot for this issue
		slots := m.paneManager.Slots()
		for i, slot := range slots {
			if slot != nil && slot.Issue.Identifier == issue.Identifier {
				if err := m.paneManager.CloseSlot(i); err != nil {
					m.statusMsg = fmt.Sprintf("Error closing slot: %v", err)
				} else {
					m.statusMsg = fmt.Sprintf("Closed slot %d (%s)", i+1, issue.Identifier)
					m.rebuildList()
				}
				return m, nil
			}
		}
		m.statusMsg = fmt.Sprintf("%s is not in a slot", issue.Identifier)
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("d", "enter"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.view = viewDetail
		m.detailIssue = issue
		contentWidth := m.width - 6
		m.detailViewport.Width = contentWidth
		m.detailViewport.Height = m.height - 6
		m.detailViewport.SetContent(m.buildDetailContent(issue, contentWidth))
		m.detailViewport.GotoTop()
		if issue.ID != m.cachedCommentID {
			return m, m.fetchCommentsCmd(issue.ID)
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.view = viewComment
		m.commentIssue = issue
		m.commentInput.SetValue("")
		m.commentInput.Focus()
		m.statusMsg = fmt.Sprintf("Comment on %s (Enter to post, Esc to cancel)", issue.Identifier)
		return m, textinput.Blink

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		issue := m.selectedIssue()
		if issue != nil && issue.URL != "" {
			openBrowser(issue.URL)
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		return m, m.buildSettingsForm()

	case key.Matches(msg, key.NewBinding(key.WithKeys("p"))):
		return m, m.showProjectPicker()

	case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.stateIssue = issue
		if len(m.workflowStates) > 0 {
			return m, m.showStatePicker()
		}
		return m, m.fetchWorkflowStates()

	case key.Matches(msg, key.NewBinding(key.WithKeys("?"))):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("S"))):
		m.view = viewSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		if m.viewer == nil {
			m.statusMsg = "Viewer not loaded yet"
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Assigning %s to you...", issue.Identifier)
		return m, m.assignToMeCmd(issue.ID, m.viewer.ID, issue.Identifier)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) updateComment(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.commentIssue = nil
		m.statusMsg = m.buildStatusLine()
		return m, nil

	case "enter":
		body := strings.TrimSpace(m.commentInput.Value())
		if body == "" {
			m.statusMsg = "Empty comment, cancelled"
			m.view = viewList
			return m, nil
		}
		issue := m.commentIssue
		m.view = viewList
		m.statusMsg = fmt.Sprintf("Posting comment on %s...", issue.Identifier)
		return m, m.postCommentCmd(issue.ID, body, issue.Identifier)
	}

	var cmd tea.Cmd
	m.commentInput, cmd = m.commentInput.Update(msg)
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "d":
		m.view = viewList
		m.detailIssue = nil
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	case "m":
		if m.detailIssue != nil {
			m.view = viewComment
			m.commentIssue = m.detailIssue
			m.commentInput.SetValue("")
			m.commentInput.Focus()
			m.statusMsg = fmt.Sprintf("Comment on %s (Enter to post, Esc to cancel)", m.detailIssue.Identifier)
			return m, textinput.Blink
		}
		return m, nil
	case "g":
		if m.detailIssue != nil && m.detailIssue.URL != "" {
			openBrowser(m.detailIssue.URL)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.detailViewport, cmd = m.detailViewport.Update(msg)
	return m, cmd
}

func (m *Model) updateLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.launchIssue = nil
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	case "enter":
		item := m.launchList.SelectedItem()
		if item == nil {
			return m, nil
		}
		opt := item.(launchOption)
		switch opt.action {
		case "prompt":
			includeComments := m.cachedCommentID == m.launchIssue.ID
			m.promptArea.SetValue(m.buildLaunchPrompt(m.launchIssue, includeComments))
			m.promptArea.SetWidth(m.width - 4)
			m.promptArea.SetHeight(m.height - 6)
			m.promptArea.Focus()
			m.view = viewPrompt
			return m, textarea.Blink
		case "blank":
			m.view = viewList
			issue := *m.launchIssue
			m.launchIssue = nil
			m.statusMsg = fmt.Sprintf("Launching Claude for %s...", issue.Identifier)
			return m, m.launchWithPromptCmd(issue, "")
		case "resume":
			m.view = viewList
			m.launchIssue = nil
			if m.paneManager == nil || opt.slotIndex < 0 {
				m.statusMsg = "Unable to focus existing session"
				return m, nil
			}
			if err := m.paneManager.FocusSlot(opt.slotIndex); err != nil {
				m.statusMsg = fmt.Sprintf("Error focusing slot %d: %v", opt.slotIndex+1, err)
				return m, nil
			}
			m.statusMsg = fmt.Sprintf("Focused existing session (slot %d)", opt.slotIndex+1)
			return m, nil
		case "existing":
			m.view = viewList
			issue := *m.launchIssue
			m.launchIssue = nil
			m.statusMsg = fmt.Sprintf("Launching Claude for %s...", issue.Identifier)
			return m, m.launchWithPromptCmd(issue, "")
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.launchList, cmd = m.launchList.Update(msg)
	return m, cmd
}

func (m *Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewLaunch
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+s":
		prompt := m.promptArea.Value()
		m.view = viewList
		issue := *m.launchIssue
		m.launchIssue = nil
		m.statusMsg = fmt.Sprintf("Launching Claude for %s...", issue.Identifier)
		return m, m.launchWithPromptCmd(issue, prompt)
	}
	var cmd tea.Cmd
	m.promptArea, cmd = m.promptArea.Update(msg)
	return m, cmd
}

func (m Model) buildLaunchPrompt(issue *Issue, includeComments bool) string {
	base := buildPrompt(*issue, m.cfg)

	if includeComments && len(m.cachedComments) > 0 {
		var b strings.Builder
		b.WriteString(base)
		b.WriteString("\n\n---\nComments:\n")
		for _, c := range m.cachedComments {
			name := c.User.DisplayName
			if name == "" {
				name = c.User.Name
			}
			fmt.Fprintf(&b, "\n@%s:\n%s\n", name, c.Body)
		}
		return b.String()
	}
	return base
}

func (m Model) launchWithPromptCmd(issue Issue, prompt string) tea.Cmd {
	return func() tea.Msg {
		wtPath, err := CreateWorktree(issue.Identifier, m.cfg)
		if err != nil {
			return worktreeCreatedMsg{err: err, identifier: issue.Identifier}
		}
		RunPostCreateHook(wtPath, m.cfg)
		return launchReadyMsg{issue: issue, wtPath: wtPath, prompt: prompt}
	}
}

func (m *Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsTabs[0] == nil {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+s":
		return m.handleSettingsCompleted()
	case "esc":
		if m.settingsFirstRun {
			return m, nil
		}
		m.settingsTabs = [3]*huh.Form{}
		m.view = viewList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	f := m.activeSettingsForm()
	form, cmd := f.Update(msg)
	if updated, ok := form.(*huh.Form); ok {
		m.settingsTabs[m.settingsActiveTab] = updated
	}

	// If the single-group form completed (Enter on last field), advance to next tab
	// If aborted (Esc on first field), go to previous tab
	active := m.activeSettingsForm()
	if active.State == huh.StateCompleted {
		if m.settingsActiveTab < len(m.settingsTabs)-1 {
			m.rebuildActiveTab()
			m.settingsActiveTab++
		} else {
			m.rebuildActiveTab()
		}
	} else if active.State == huh.StateAborted {
		if m.settingsActiveTab > 0 {
			m.rebuildActiveTab()
			m.settingsActiveTab--
		} else if !m.settingsFirstRun {
			m.settingsTabs = [3]*huh.Form{}
			m.view = viewList
			return m, nil
		} else {
			m.rebuildActiveTab()
		}
	}

	return m, cmd
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
	// Build a map of issue identifier → slot info
	slotMap := make(map[string]*WorktreeSlot)
	if m.paneManager != nil {
		slots := m.paneManager.Slots()
		for _, slot := range slots {
			if slot != nil {
				slotMap[slot.Issue.Identifier] = slot
			}
		}
	}

	items := make([]list.Item, len(m.issues))
	for i, issue := range m.issues {
		branch := m.cfg.BranchPrefix + strings.ToLower(issue.Identifier)
		hasWt := m.worktreeBranches[branch]

		item := issueItem{
			issue:       issue,
			hasWorktree: hasWt,
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
	if m.useCmux && m.paneManager != nil {
		active := m.paneManager.ActiveCount()
		parts = append(parts, fmt.Sprintf("slots: %d/%d", active, m.cfg.MaxSlots))
	}
	return strings.Join(parts, " | ")
}

// --- View ---

func (m Model) View() string {
	switch m.view {
	case viewSettings:
		return m.viewSettings()
	case viewComment:
		return m.viewComment()
	case viewDetail:
		return m.viewDetail()
	case viewLaunch:
		return m.viewLaunch()
	case viewPrompt:
		return m.viewPrompt()
	case viewProjectPicker:
		return m.viewPicker("Select Project", m.projectForm)
	case viewStatePicker:
		return m.viewPicker("Transition State", m.stateForm)
	case viewFilterPicker:
		return m.viewPicker("Filter Issues", m.filterForm)
	case viewSearch:
		return m.viewSearchInput()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	slotBar := m.renderSlotBar()
	content := m.list.View()
	status := statusBarStyle.Render(m.statusMsg)
	helpBar := m.help.View(m.keys)
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, slotBar, content, status, helpBar),
	)
}

func (m Model) viewDetail() string {
	identifier := ""
	if m.detailIssue != nil {
		identifier = m.detailIssue.Identifier
	}

	header := titleStyle.Render(fmt.Sprintf("Issue: %s", identifier))
	body := m.detailViewport.View()

	scrollPct := fmt.Sprintf("%3.f%%", m.detailViewport.ScrollPercent()*100)
	status := statusBarStyle.Render(fmt.Sprintf(
		"%s | d/esc:back  j/k:scroll  m:comment  g:open  q:quit", scrollPct))

	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, header, body, status),
	)
}

func (m Model) renderSlotBar() string {
	if !m.useCmux || m.paneManager == nil {
		return ""
	}

	slots := m.paneManager.Slots()
	parts := make([]string, m.cfg.MaxSlots)
	for i := 0; i < m.cfg.MaxSlots; i++ {
		slot := slots[i]
		if slot == nil {
			parts[i] = slotEmptyStyle.Render(fmt.Sprintf("[%d] empty", i+1))
		} else {
			var style lipgloss.Style
			switch slot.Status {
			case AgentRunning:
				style = slotRunningStyle
			case AgentWaiting:
				style = slotWaitingStyle
			case AgentIdle:
				style = slotIdleStyle
			default:
				style = slotEmptyStyle
			}
			parts[i] = style.Render(fmt.Sprintf("[%d] %s %s (%s)",
				i+1, slot.Status.String(), slot.Issue.Identifier, slot.Status.Label()))
		}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, "  "))
}

func (m Model) buildDetailContent(issue *Issue, width int) string {
	wrap := func(s string) string {
		return lipgloss.NewStyle().Width(width).Render(s)
	}
	dim := commentDimStyle.Render
	sectionHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Render
	blockerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Render

	field := func(label, value string) string {
		return dim(fmt.Sprintf("%-12s", label)) + value + "\n"
	}

	var b strings.Builder

	// Header
	b.WriteString(issueIdentStyle.Render(issue.Identifier))
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")).Render(issue.State.Name))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Width(width).Render(issue.Title))
	b.WriteString("\n\n")

	// Metadata
	if issue.Project != nil {
		b.WriteString(field("Project", issue.Project.Name))
	}
	if issue.Cycle != nil {
		b.WriteString(field("Cycle", issue.Cycle.Name))
	}
	if issue.Assignee != nil {
		name := issue.Assignee.DisplayName
		if name == "" {
			name = issue.Assignee.Name
		}
		b.WriteString(field("Assignee", name))
	}
	priNames := map[int]string{0: "None", 1: "Urgent", 2: "High", 3: "Medium", 4: "Low"}
	b.WriteString(field("Priority", priNames[issue.Priority]))
	if issue.Estimate != nil {
		b.WriteString(field("Estimate", fmt.Sprintf("%.0f pts", *issue.Estimate)))
	}
	if issue.DueDate != nil {
		dueStr := *issue.DueDate
		if t, err := time.Parse("2006-01-02", *issue.DueDate); err == nil {
			days := int(time.Until(t).Hours() / 24)
			switch {
			case days < 0:
				dueStr = blockerStyle(fmt.Sprintf("%s (OVERDUE by %dd)", *issue.DueDate, -days))
			case days <= 3:
				dueStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")).Render(
					fmt.Sprintf("%s (%dd)", *issue.DueDate, days))
			default:
				dueStr = fmt.Sprintf("%s (%dd)", *issue.DueDate, days)
			}
		}
		b.WriteString(field("Due", dueStr))
	}
	if len(issue.Labels.Nodes) > 0 {
		labels := make([]string, len(issue.Labels.Nodes))
		for i, l := range issue.Labels.Nodes {
			labels[i] = l.Name
		}
		b.WriteString(field("Labels", wrap(strings.Join(labels, ", "))))
	}
	if issue.CreatedAt != "" {
		b.WriteString(field("Created", relativeTime(issue.CreatedAt)))
	}
	if issue.UpdatedAt != "" {
		b.WriteString(field("Updated", relativeTime(issue.UpdatedAt)))
	}
	if issue.BranchName != "" {
		b.WriteString(field("Branch", lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Render(issue.BranchName)))
	}
	if issue.URL != "" {
		b.WriteString(field("URL", linkStyle(issue.URL)))
	}

	// Relations
	if len(issue.Relations.Nodes) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionHeader("Relations"))
		b.WriteString("\n")
		for _, r := range issue.Relations.Nodes {
			prefix := r.Type
			style := dim
			switch r.Type {
			case "blocks":
				prefix = "Blocking"
			case "is blocked by", "blocked":
				prefix = "Blocked by"
				style = blockerStyle
			case "related":
				prefix = "Related"
			case "duplicate":
				prefix = "Duplicate"
			}
			b.WriteString(fmt.Sprintf("  %s %s\n",
				style(prefix+":"),
				linkStyle(r.RelatedIssue.Identifier)+dim(" "+r.RelatedIssue.Title)))
		}
	}

	// Parent
	if issue.Parent != nil {
		b.WriteString("\n")
		b.WriteString(field("Parent", linkStyle(issue.Parent.Identifier)+dim(" "+issue.Parent.Title)))
	}

	// Sub-issues
	if len(issue.Children.Nodes) > 0 {
		b.WriteString("\n")
		completed := 0
		for _, child := range issue.Children.Nodes {
			if child.State.Type == "completed" {
				completed++
			}
		}
		b.WriteString(sectionHeader(fmt.Sprintf("Sub-issues [%d/%d]", completed, len(issue.Children.Nodes))))
		b.WriteString("\n")
		for _, child := range issue.Children.Nodes {
			icon := statusIcon(child.State.Type)
			b.WriteString(fmt.Sprintf("  %s %s %s\n", icon, linkStyle(child.Identifier), dim(child.Title)))
		}
	}

	// Description
	if issue.Description != "" {
		b.WriteString("\n")
		b.WriteString(sectionHeader("Description"))
		b.WriteString("\n")
		b.WriteString(wrap(issue.Description))
		b.WriteString("\n")
	}

	// Comments
	if m.cachedCommentID == issue.ID && len(m.cachedComments) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionHeader(fmt.Sprintf("Comments (%d)", len(m.cachedComments))))
		b.WriteString("\n")
		for _, c := range m.cachedComments {
			name := c.User.DisplayName
			if name == "" {
				name = c.User.Name
			}
			isMe := m.viewer != nil && c.User.ID == m.viewer.ID
			nameStyle := dim
			if isMe {
				nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true).Render
			}
			b.WriteString(fmt.Sprintf("\n%s %s\n", nameStyle(name+":"), dim(relativeTime(c.CreatedAt))))
			b.WriteString(wrap(c.Body))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func relativeTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func (m Model) viewLaunch() string {
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			m.launchList.View(),
		),
	)
}

func (m Model) viewPrompt() string {
	identifier := ""
	if m.launchIssue != nil {
		identifier = m.launchIssue.Identifier
	}
	header := titleStyle.Render(fmt.Sprintf("Prompt for %s", identifier))
	status := statusBarStyle.Render("ctrl+s:launch  esc:back")
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, header, m.promptArea.View(), status),
	)
}

func (m Model) viewComment() string {
	identifier := ""
	if m.commentIssue != nil {
		identifier = m.commentIssue.Identifier
	}

	listView := m.list.View()
	commentBar := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("#7C3AED")).
		Render(fmt.Sprintf("💬 Comment on %s: ", identifier)) +
		m.commentInput.View()

	status := statusBarStyle.Render("[Enter] post  [Esc] cancel")
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, listView, commentBar, status),
	)
}

func (m Model) viewSettings() string {
	if m.settingsTabs[0] == nil {
		return ""
	}
	header := titleStyle.Render("Settings")
	tabBar := m.renderSettingsTabBar()
	body := m.activeSettingsForm().View()
	help := statusBarStyle.Render("Enter/Tab: next field  Shift+Tab: prev field  Ctrl+S: save  Esc: cancel")
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, header, tabBar, "", body, "", help),
	)
}

// --- Project & State Pickers ---

// --- Settings Tabs ---

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888")).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(lipgloss.Color("#444")).
				Padding(0, 2)
)

func (m *Model) activeSettingsForm() *huh.Form {
	return m.settingsTabs[m.settingsActiveTab]
}

func (m *Model) initSettingsForm() {
	m.settingsAPIKey = m.cfg.LinearAPIKey
	m.settingsTeamKey = m.cfg.TeamKey
	m.settingsWtBase = m.cfg.WorktreeBase
	m.settingsCopyFiles = strings.Join(m.cfg.CopyFiles, ", ")
	m.settingsCopyDirs = strings.Join(m.cfg.CopyDirs, ", ")
	m.settingsClaudeCmd = m.cfg.ClaudeCommand
	m.settingsClaudeArgs = m.cfg.ClaudeArgs
	m.settingsBranch = m.cfg.BranchPrefix
	m.settingsMaxSlots = m.cfg.MaxSlots
	m.settingsHook = m.cfg.PostCreateHook
	m.settingsPrompt = m.cfg.PromptTemplate
	m.settingsActiveTab = 0

	w := m.width - 4
	if w < 60 {
		w = 60
	}

	m.settingsTabNames = [3]string{"Credentials", "Worktree", "Launch"}
	for i := range m.settingsTabs {
		m.settingsTabs[i] = m.buildTab(i, w)
	}

	m.view = viewSettings
}

func (m *Model) buildTab(index, w int) *huh.Form {
	switch index {
	case 0:
		return huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Linear API Key").
					Description("Personal API key from Linear Settings > API. Stored securely in your OS keychain, never written to the config file.").
					Placeholder("lin_api_...").
					EchoMode(huh.EchoModePassword).
					Value(&m.settingsAPIKey),
				huh.NewInput().
					Title("Team Key").
					Description("The short prefix for your team's issues (e.g. TSCODE). Find it in the URL: linear.app/TEAMKEY/...").
					Placeholder("MYTEAM").
					Value(&m.settingsTeamKey),
			),
		).WithWidth(w).WithShowHelp(false).WithShowErrors(true)
	case 1:
		return huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Worktree Base Directory").
					Description("Where new git worktrees are created, relative to the repo root. Each issue gets a subdirectory here.").
					Placeholder("../worktrees").
					Value(&m.settingsWtBase),
				huh.NewInput().
					Title("Files to Copy").
					Description("Comma-separated list of files copied from the main repo into each new worktree (e.g. env files, configs).").
					Placeholder(".env, .envrc").
					Value(&m.settingsCopyFiles),
				huh.NewInput().
					Title("Directories to Copy").
					Description("Comma-separated list of directories copied into each new worktree (e.g. .claude for Claude Code settings).").
					Placeholder(".claude").
					Value(&m.settingsCopyDirs),
				huh.NewInput().
					Title("Branch Prefix").
					Description("Prefix added to git branch names when creating worktrees. Issue TSCODE-123 becomes feature/tscode-123.").
					Placeholder("feature/").
					Value(&m.settingsBranch),
			),
		).WithWidth(w).WithShowHelp(false).WithShowErrors(true)
	default:
		return huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Claude Command").
					Description("The command used to launch Claude Code. Change this if claude is installed at a custom path.").
					Placeholder("claude").
					Value(&m.settingsClaudeCmd).
					Validate(func(s string) error {
						s = strings.TrimSpace(s)
						if s == "" {
							return nil
						}
						return validateClaudeCommand(s)
					}),
				huh.NewInput().
					Title("Claude Args").
					Description("Extra flags appended to every Claude launch (e.g. --model sonnet, --verbose, --allowedTools).").
					Value(&m.settingsClaudeArgs),
				huh.NewInput().
					Title("Post-Create Hook").
					Description("Shell command that runs inside the worktree directory after creation. Use for setup tasks like installing dependencies.").
					Placeholder("npm install && direnv allow").
					Value(&m.settingsHook),
				huh.NewText().
					Title("Prompt Template").
					Description("Custom prompt sent to Claude on launch. Supports Go template variables: {{.Identifier}}, {{.Title}}, {{.Description}}. Leave empty for the default prompt.").
					Value(&m.settingsPrompt),
				huh.NewSelect[int]().
					Title("Max Slots").
					Description("Maximum number of concurrent Claude sessions in the E-layout. Only applies when running inside cmux.").
					Options(
						huh.NewOption("2 slots", 2),
						huh.NewOption("3 slots", 3),
						huh.NewOption("4 slots", 4),
					).
					Value(&m.settingsMaxSlots),
			),
		).WithWidth(w).WithShowHelp(false).WithShowErrors(true)
	}
}

func (m *Model) buildSettingsForm() tea.Cmd {
	m.initSettingsForm()
	cmds := make([]tea.Cmd, len(m.settingsTabs))
	for i := range m.settingsTabs {
		cmds[i] = m.settingsTabs[i].Init()
	}
	return tea.Batch(cmds...)
}

func (m *Model) rebuildActiveTab() {
	w := m.width - 4
	if w < 60 {
		w = 60
	}
	m.settingsTabs[m.settingsActiveTab] = m.buildTab(m.settingsActiveTab, w)
	m.settingsTabs[m.settingsActiveTab].Init()
}

func (m Model) renderSettingsTabBar() string {
	var tabs []string
	for i, name := range m.settingsTabNames {
		if i == m.settingsActiveTab {
			tabs = append(tabs, activeTabStyle.Render(name))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m *Model) handleSettingsCompleted() (tea.Model, tea.Cmd) {
	apiKey := strings.TrimSpace(m.settingsAPIKey)
	teamKey := strings.TrimSpace(m.settingsTeamKey)

	if apiKey == "" || teamKey == "" {
		m.statusMsg = "API key and team key are required"
		m.settingsActiveTab = 0
		return m, nil
	}

	newCfg := m.cfg
	newCfg.LinearAPIKey = apiKey
	newCfg.TeamKey = teamKey
	newCfg.WorktreeBase = strings.TrimSpace(m.settingsWtBase)
	newCfg.CopyFiles = splitComma(m.settingsCopyFiles)
	newCfg.CopyDirs = splitComma(m.settingsCopyDirs)
	newCfg.ClaudeCommand = strings.TrimSpace(m.settingsClaudeCmd)
	newCfg.ClaudeArgs = strings.TrimSpace(m.settingsClaudeArgs)
	newCfg.BranchPrefix = strings.TrimSpace(m.settingsBranch)
	newCfg.MaxSlots = m.settingsMaxSlots
	newCfg.PostCreateHook = strings.TrimSpace(m.settingsHook)
	newCfg.PromptTemplate = m.settingsPrompt

	if newCfg.WorktreeBase == "" {
		newCfg.WorktreeBase = "../worktrees"
	}
	if newCfg.ClaudeCommand == "" {
		newCfg.ClaudeCommand = "claude"
	}
	if newCfg.BranchPrefix == "" {
		newCfg.BranchPrefix = "feature/"
	}

	m.settingsTabs = [3]*huh.Form{}

	if teamKey != m.cfg.TeamKey || newCfg.TeamID == "" {
		m.cfg = newCfg
		m.statusMsg = "Resolving team..."
		return m, m.resolveTeamCmd(apiKey, teamKey)
	}

	if err := SaveConfig(newCfg); err != nil {
		m.statusMsg = fmt.Sprintf("Save error: %v", err)
		m.view = viewList
		return m, nil
	}

	m.cfg = newCfg
	m.view = viewList
	m.settingsFirstRun = false
	m.statusMsg = "Settings saved."
	m.updateListTitle()
	if m.paneManager != nil {
		m.paneManager = NewPaneManager(m.cmuxClient, m.cfg.MaxSlots)
	}
	cmds := []tea.Cmd{m.fetchIssues(), m.fetchWorktrees()}
	if m.useCmux {
		cmds = append(cmds, m.startStatusPoll())
	}
	return m, tea.Batch(cmds...)
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// --- Project & State Pickers ---

func (m *Model) showProjectPicker() tea.Cmd {
	options := []huh.Option[string]{
		huh.NewOption("All issues", ""),
		huh.NewOption("No project", "none"),
	}
	for _, p := range m.projects {
		label := p.Name
		if p.Progress > 0 {
			label = fmt.Sprintf("%s (%.0f%%)", p.Name, p.Progress*100)
		}
		options = append(options, huh.NewOption(label, p.ID))
	}

	m.projectForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("project").
				Title("Filter by project").
				Options(options...),
		),
	).WithWidth(50).WithShowHelp(false).WithShowErrors(false)
	m.view = viewProjectPicker
	return m.projectForm.Init()
}

func (m *Model) showStatePicker() tea.Cmd {
	if m.stateIssue == nil {
		return nil
	}
	options := make([]huh.Option[string], 0, len(m.workflowStates))
	for _, s := range m.workflowStates {
		label := fmt.Sprintf("%s (%s)", s.Name, s.Type)
		options = append(options, huh.NewOption(label, s.ID))
	}

	m.stateForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("state").
				Title(fmt.Sprintf("Transition %s", m.stateIssue.Identifier)).
				Options(options...),
		),
	).WithWidth(50).WithShowHelp(false).WithShowErrors(false)
	m.view = viewStatePicker
	return m.stateForm.Init()
}

func (m *Model) handleProjectSelected() (tea.Model, tea.Cmd) {
	selected := m.projectForm.GetString("project")
	m.projectForm = nil
	m.view = viewList

	switch selected {
	case "":
		m.projectFilter = nil
		m.projectName = ""
	case "none":
		s := "none"
		m.projectFilter = &s
		m.projectName = "No project"
	default:
		m.projectFilter = &selected
		for _, p := range m.projects {
			if p.ID == selected {
				m.projectName = p.Name
				break
			}
		}
	}

	m.updateListTitle()
	m.loading = true
	m.loadingLabel = "Loading..."
	return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)
}

func (m *Model) updateListTitle() {
	title := m.cfg.TeamKey
	if m.projectName != "" {
		title = fmt.Sprintf("%s > %s", m.cfg.TeamKey, m.projectName)
	}
	if m.filter != FilterAssigned {
		title = fmt.Sprintf("%s [%s]", title, m.filter.String())
	}
	m.list.Title = title
}

func (m *Model) handleStateSelected() (tea.Model, tea.Cmd) {
	selected := m.stateForm.GetString("state")
	issue := m.stateIssue
	m.stateForm = nil
	m.stateIssue = nil
	m.view = viewList

	if selected != "" && issue != nil {
		m.statusMsg = fmt.Sprintf("Updating state for %s...", issue.Identifier)
		return m, m.changeStateCmd(issue.ID, selected, issue.Identifier)
	}
	return m, nil
}

func (m *Model) showFilterPicker() tea.Cmd {
	options := []huh.Option[string]{
		huh.NewOption("Assigned to me", "assigned"),
		huh.NewOption("All issues", "all"),
		huh.NewOption("Todo", "todo"),
		huh.NewOption("In Progress", "inprogress"),
		huh.NewOption("Unassigned", "unassigned"),
	}

	m.filterForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("filter").
				Title("Filter issues").
				Options(options...),
		),
	).WithWidth(50).WithShowHelp(false).WithShowErrors(false)
	m.view = viewFilterPicker
	return m.filterForm.Init()
}

func (m *Model) handleFilterSelected() (tea.Model, tea.Cmd) {
	selected := m.filterForm.GetString("filter")
	m.filterForm = nil
	m.view = viewList

	switch selected {
	case "assigned":
		m.filter = FilterAssigned
	case "all":
		m.filter = FilterAll
	case "todo":
		m.filter = FilterTodo
	case "inprogress":
		m.filter = FilterInProgress
	case "unassigned":
		m.filter = FilterUnassigned
	default:
		return m, nil
	}

	m.updateListTitle()
	m.loading = true
	m.loadingLabel = "Loading..."
	return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)
}

func (m *Model) updateFilterPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.filterForm = nil
		return m, nil
	}

	form, cmd := m.filterForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.filterForm = f
	}

	if m.filterForm.State == huh.StateCompleted {
		return m.handleFilterSelected()
	}

	return m, cmd
}

func (m *Model) updateProjectPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.projectForm = nil
		return m, nil
	}

	form, cmd := m.projectForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.projectForm = f
	}

	if m.projectForm.State == huh.StateCompleted {
		return m.handleProjectSelected()
	}

	return m, cmd
}

func (m *Model) updateStatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.stateForm = nil
		m.stateIssue = nil
		return m, nil
	}

	form, cmd := m.stateForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.stateForm = f
	}

	if m.stateForm.State == huh.StateCompleted {
		return m.handleStateSelected()
	}

	return m, cmd
}

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		if m.searching && m.savedIssues != nil {
			m.issues = m.savedIssues
			m.savedIssues = nil
			m.searching = false
			m.searchTerm = ""
			m.rebuildList()
			m.statusMsg = m.buildStatusLine()
		}
		return m, nil

	case "enter":
		term := strings.TrimSpace(m.searchInput.Value())
		if term == "" {
			m.view = viewList
			return m, nil
		}
		m.searchTerm = term
		if !m.searching {
			m.savedIssues = m.issues
		}
		m.view = viewList
		m.loading = true
		m.loadingLabel = "Searching..."
		return m, tea.Batch(m.searchIssuesCmd(term), m.spinner.Tick)
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m Model) searchIssuesCmd(term string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		issues, _, err := client.SearchIssues(term, m.cfg.TeamID, 50, "")
		return searchResultsMsg{issues: issues, err: err}
	}
}

func (m Model) viewSearchInput() string {
	listView := m.list.View()
	searchBar := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("#7C3AED")).
		Render("Search: ") +
		m.searchInput.View()

	status := statusBarStyle.Render("[Enter] search  [Esc] cancel")
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, listView, searchBar, status),
	)
}

func (m Model) viewPicker(title string, form *huh.Form) string {
	if form == nil {
		return ""
	}
	header := titleStyle.Render(title)
	body := form.View()
	status := statusBarStyle.Render("[Enter] select  [Esc] cancel")
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		setupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, status)),
	)
}

func openBrowser(url string) {
	if !strings.HasPrefix(url, "https://") {
		return
	}
	for _, cmd := range []string{"open", "xdg-open", "wslview"} {
		if err := execCommand(cmd, url).Start(); err == nil {
			return
		}
	}
}

func execCommand(name string, args ...string) *osexec.Cmd {
	return osexec.Command(name, args...)
}
