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
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

	detailStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#444")).
			Padding(1, 2).
			Width(40)

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

func (i issueItem) Description() string { return "" }
func (i issueItem) FilterValue() string {
	return i.issue.Identifier + " " + i.issue.Title
}

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
	err        error
}

type teamsLoadedMsg struct {
	teams []Team
	err   error
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

type statusPollMsg struct{}

// --- App state ---

type viewMode int

const (
	viewList viewMode = iota
	viewSetup
	viewComment
)

type setupField int

const (
	fieldAPIKey setupField = iota
	fieldTeamKey
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
	showDetail       bool
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
	cachedComments   []Comment
	cachedCommentID  string

	// Detail viewport
	detailViewport viewport.Model

	// Help + spinner
	help     help.Model
	keys     keyMap
	spinner  spinner.Model
	loading  bool

	// Setup fields
	setupField   setupField
	apiKeyInput  textinput.Model
	teamKeyInput textinput.Model
}

// keyMap defines keybindings for the help component.
type keyMap struct {
	Navigate key.Binding
	Claude   key.Binding
	Worktree key.Binding
	Close    key.Binding
	Comment  key.Binding
	Detail   key.Binding
	Filter   key.Binding
	Open     key.Binding
	Refresh  key.Binding
	Search   key.Binding
	Setup    key.Binding
	Quit     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Navigate: key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "navigate")),
		Claude:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "claude+worktree")),
		Worktree: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "worktree")),
		Close:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close slot")),
		Comment:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "comment")),
		Detail:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "detail")),
		Filter:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "filter")),
		Open:     key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "open")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Setup:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "setup")),
		Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Claude, k.Worktree, k.Comment, k.Detail, k.Filter, k.Close, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Navigate, k.Claude, k.Worktree, k.Close},
		{k.Comment, k.Detail, k.Filter, k.Search},
		{k.Open, k.Refresh, k.Setup, k.Quit},
	}
}

func NewModel(cfg Config) Model {
	// Issue list
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "linear-worktree"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	// Setup inputs
	apiKey := textinput.New()
	apiKey.Placeholder = "lin_api_..."
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.Focus()

	teamKey := textinput.New()
	teamKey.Placeholder = "TSCODE"

	// Comment input
	commentIn := textinput.New()
	commentIn.Placeholder = "Type your comment..."
	commentIn.CharLimit = 2000

	// Check for cmux
	cmuxClient := NewCmuxClient()
	useCmux := cmuxClient.Available()

	var pm *PaneManager
	if useCmux {
		pm = NewPaneManager(cmuxClient, cfg.MaxSlots)
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

	return Model{
		cfg:              cfg,
		list:             l,
		worktreeBranches: make(map[string]bool),
		filter:           FilterAssigned,
		view:             viewList,
		apiKeyInput:      apiKey,
		teamKeyInput:     teamKey,
		commentInput:     commentIn,
		cmuxClient:       cmuxClient,
		paneManager:      pm,
		useCmux:          useCmux,
		help:             h,
		keys:             defaultKeyMap(),
		spinner:          sp,
		detailViewport:   vp,
	}
}

func (m Model) Init() tea.Cmd {
	if m.cfg.NeedsSetup() {
		m.view = viewSetup
		return textinput.Blink
	}
	cmds := []tea.Cmd{
		m.fetchIssues(),
		m.fetchWorktrees(),
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
		issues, err := client.GetIssues(m.cfg.TeamID, m.filter)
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

func (m Model) createWorktreeCmd(issue Issue) tea.Cmd {
	return func() tea.Msg {
		path, err := CreateWorktree(issue.Identifier, m.cfg)
		return worktreeCreatedMsg{
			path:       path,
			identifier: issue.Identifier,
			err:        err,
		}
	}
}

func (m Model) launchClaudeCmd(wtPath string, issue Issue) tea.Cmd {
	return func() tea.Msg {
		err := LaunchClaude(wtPath, issue, m.cfg)
		return claudeLaunchedMsg{identifier: issue.Identifier, err: err}
	}
}

func (m Model) openCmuxSlotCmd(issue Issue, wtPath string) tea.Cmd {
	return func() tea.Msg {
		slot, err := m.paneManager.OpenSlot(issue, wtPath, m.cfg)
		if err != nil {
			return cmuxSlotOpenedMsg{err: err, identifier: issue.Identifier}
		}
		return cmuxSlotOpenedMsg{slotIdx: slot.Index, identifier: issue.Identifier}
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

func (m Model) resolveTeamCmd(apiKey, teamKey string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(apiKey)
		teams, err := client.GetTeams()
		if err != nil {
			return teamsLoadedMsg{err: err}
		}

		for _, t := range teams {
			if strings.EqualFold(t.Key, teamKey) {
				cfg := m.cfg
				cfg.LinearAPIKey = apiKey
				cfg.TeamID = t.ID
				cfg.TeamKey = t.Key
				if err := SaveConfig(cfg); err != nil {
					return teamsLoadedMsg{err: err}
				}
				return setupCompleteMsg{cfg: cfg}
			}
		}

		available := make([]string, len(teams))
		for i, t := range teams {
			available[i] = t.Key
		}
		return teamsLoadedMsg{
			err: fmt.Errorf("team '%s' not found. Available: %s",
				teamKey, strings.Join(available, ", ")),
		}
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
		listWidth := msg.Width - 2
		if m.showDetail {
			listWidth = msg.Width - 42
		}
		m.list.SetSize(listWidth, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys when list is filtering
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		switch m.view {
		case viewSetup:
			return m.updateSetup(msg)
		case viewComment:
			return m.updateComment(msg)
		default:
			return m.updateList(msg)
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
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
		// Find the issue for launching
		for _, issue := range m.issues {
			if issue.Identifier == msg.identifier {
				if m.useCmux && m.paneManager != nil {
					return m, tea.Batch(
						m.fetchWorktrees(),
						m.openCmuxSlotCmd(issue, msg.path),
					)
				}
				return m, tea.Batch(
					m.fetchWorktrees(),
					m.launchClaudeCmd(msg.path, issue),
				)
			}
		}
		return m, m.fetchWorktrees()

	case cmuxSlotOpenedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error opening slot: %v", msg.err)
			// Fall back to tmux
			for _, issue := range m.issues {
				if issue.Identifier == msg.identifier {
					wtPath := m.cfg.WorktreeBase + "/" + strings.ToLower(msg.identifier)
					return m, m.launchClaudeCmd(wtPath, issue)
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
		}
		return m, nil

	case statusPollMsg:
		if m.paneManager != nil {
			m.paneManager.PollStatus()
			m.rebuildList()
		}
		return m, m.startStatusPoll()

	case setupCompleteMsg:
		m.cfg = msg.cfg
		m.view = viewList
		m.statusMsg = "Setup complete!"
		cmds := []tea.Cmd{m.fetchIssues(), m.fetchWorktrees()}
		if m.useCmux {
			cmds = append(cmds, m.startStatusPoll())
		}
		return m, tea.Batch(cmds...)

	case teamsLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Setup error: %v", msg.err)
		}
		return m, nil
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
		m.loading = true
		m.statusMsg = m.spinner.View() + " Loading..."
		return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		m.loading = true
		m.statusMsg = m.spinner.View() + " Refreshing..."
		return m, tea.Batch(m.fetchIssues(), m.fetchWorktrees(), m.spinner.Tick)

	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		// Check slot capacity for cmux mode
		if m.useCmux && m.paneManager != nil && m.paneManager.ActiveCount() >= m.cfg.MaxSlots {
			m.statusMsg = fmt.Sprintf("All %d slots full. Close a worktree first (x key).", m.cfg.MaxSlots)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Creating worktree + launching Claude for %s...", issue.Identifier)
		return m, m.createWorktreeCmd(*issue)

	case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Creating worktree for %s...", issue.Identifier)
		return m, func() tea.Msg {
			_, err := CreateWorktree(issue.Identifier, m.cfg)
			if err != nil {
				return worktreeCreatedMsg{err: err, identifier: issue.Identifier}
			}
			return worktreesLoadedMsg{branches: m.worktreeBranches}
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

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		m.showDetail = !m.showDetail
		listWidth := m.width - 2
		if m.showDetail {
			listWidth = m.width - 42
		}
		m.list.SetSize(listWidth, m.height-4)
		// Load comments for the selected issue
		if m.showDetail {
			if issue := m.selectedIssue(); issue != nil {
				return m, m.fetchCommentsCmd(issue.ID)
			}
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
		m.view = viewSetup
		m.apiKeyInput.SetValue("")
		m.teamKeyInput.SetValue("")
		m.setupField = fieldAPIKey
		m.apiKeyInput.Focus()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	// When highlight changes and detail panel is open, fetch comments
	if m.showDetail {
		if issue := m.selectedIssue(); issue != nil && issue.ID != m.cachedCommentID {
			return m, m.fetchCommentsCmd(issue.ID)
		}
	}

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

func (m *Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if !m.cfg.NeedsSetup() {
			m.view = viewList
		}
		return m, nil

	case "tab", "enter":
		if m.setupField == fieldAPIKey {
			m.setupField = fieldTeamKey
			m.apiKeyInput.Blur()
			m.teamKeyInput.Focus()
			return m, textinput.Blink
		}
		// Submit
		apiKey := strings.TrimSpace(m.apiKeyInput.Value())
		teamKey := strings.TrimSpace(m.teamKeyInput.Value())
		if apiKey == "" || teamKey == "" {
			m.statusMsg = "Both fields required"
			return m, nil
		}
		m.statusMsg = "Verifying..."
		return m, m.resolveTeamCmd(apiKey, teamKey)
	}

	var cmd tea.Cmd
	if m.setupField == fieldAPIKey {
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	} else {
		m.teamKeyInput, cmd = m.teamKeyInput.Update(msg)
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
	parts := []string{
		fmt.Sprintf("%d issues", len(m.issues)),
		m.filter.String(),
	}
	if m.useCmux && m.paneManager != nil {
		active := m.paneManager.ActiveCount()
		parts = append(parts, fmt.Sprintf("slots: %d/%d", active, m.cfg.MaxSlots))
	}
	parts = append(parts, "c:claude w:wt m:comment d:detail x:close tab:filter")
	return strings.Join(parts, " | ")
}

// --- View ---

func (m Model) View() string {
	switch m.view {
	case viewSetup:
		return m.viewSetup()
	case viewComment:
		return m.viewComment()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	// Slot indicators at top
	slotBar := m.renderSlotBar()

	listView := m.list.View()

	var content string
	if m.showDetail {
		detail := m.renderDetail()
		content = lipgloss.JoinHorizontal(lipgloss.Top, listView, detail)
	} else {
		content = listView
	}

	status := statusBarStyle.Render(m.statusMsg)
	helpBar := m.help.View(m.keys)
	return appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, slotBar, content, status, helpBar),
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

func (m Model) renderDetail() string {
	issue := m.selectedIssue()
	if issue == nil {
		return detailStyle.Render("No issue selected")
	}

	var b strings.Builder
	b.WriteString(issueIdentStyle.Render(issue.Identifier))
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")).Render(issue.State.Name))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(issue.Title))
	b.WriteString("\n\n")

	if issue.Assignee != nil {
		name := issue.Assignee.DisplayName
		if name == "" {
			name = issue.Assignee.Name
		}
		b.WriteString(commentDimStyle.Render("Assignee: "))
		b.WriteString(name + "\n")
	}

	priNames := map[int]string{0: "None", 1: "Urgent", 2: "High", 3: "Medium", 4: "Low"}
	b.WriteString(commentDimStyle.Render("Priority: "))
	b.WriteString(priNames[issue.Priority] + "\n")

	if len(issue.Labels.Nodes) > 0 {
		labels := make([]string, len(issue.Labels.Nodes))
		for i, l := range issue.Labels.Nodes {
			labels[i] = l.Name
		}
		b.WriteString(commentDimStyle.Render("Labels: "))
		b.WriteString(strings.Join(labels, ", ") + "\n")
	}

	if issue.BranchName != "" {
		b.WriteString(commentDimStyle.Render("Branch: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Render(issue.BranchName) + "\n")
	}

	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 400 {
			desc = desc[:400] + "..."
		}
		b.WriteString("\n")
		b.WriteString(commentDimStyle.Render("Description:\n"))
		b.WriteString(desc + "\n")
	}

	// Show comments
	if m.cachedCommentID == issue.ID && len(m.cachedComments) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Render("Comments"))
		b.WriteString("\n")
		// Show last 5 comments
		start := 0
		if len(m.cachedComments) > 5 {
			start = len(m.cachedComments) - 5
		}
		for _, c := range m.cachedComments[start:] {
			name := c.User.DisplayName
			if name == "" {
				name = c.User.Name
			}
			b.WriteString(commentDimStyle.Render(fmt.Sprintf("\n%s:", name)))
			b.WriteString("\n")
			body := c.Body
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			b.WriteString(body + "\n")
		}
	}

	m.detailViewport.Width = 38
	m.detailViewport.Height = m.height - 8
	m.detailViewport.SetContent(b.String())
	return detailStyle.Height(m.height - 6).Render(m.detailViewport.View())
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

func (m Model) viewSetup() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("linear-worktree setup"))
	b.WriteString("\n\n")
	b.WriteString("Linear API Key:\n")
	b.WriteString(m.apiKeyInput.View())
	b.WriteString("\n\n")
	b.WriteString("Team Key (e.g. TSCODE):\n")
	b.WriteString(m.teamKeyInput.View())
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render(
		"[Tab] next field  [Enter] save  [Esc] cancel",
	))
	if m.statusMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(m.statusMsg))
	}

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		setupStyle.Render(b.String()),
	)
}

func openBrowser(url string) {
	for _, cmd := range []string{"open", "xdg-open", "wslview"} {
		if err := execCommand(cmd, url).Start(); err == nil {
			return
		}
	}
}

func execCommand(name string, args ...string) *osexec.Cmd {
	return osexec.Command(name, args...)
}
