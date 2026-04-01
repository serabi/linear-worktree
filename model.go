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

func (i issueItem) Description() string { return "" }
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

// --- App state ---

type viewMode int

const (
	viewList viewMode = iota
	viewSetup
	viewComment
	viewDetail
	viewLaunch
	viewPrompt
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
	cachedComments   []Comment
	cachedCommentID  string

	// Detail viewport
	detailViewport viewport.Model

	// Help + spinner
	help     help.Model
	keys     keyMap
	spinner  spinner.Model
	loading  bool

	// Launch menu + prompt editor
	launchIssue *Issue
	launchList  list.Model
	promptArea  textarea.Model

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
		Detail:   key.NewBinding(key.WithKeys("d", "enter"), key.WithHelp("enter/d", "detail")),
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
		launchList:       ll,
		promptArea:       ta,
	}
	if cfg.NeedsSetup() {
		m.view = viewSetup
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.cfg.NeedsSetup() {
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
		case viewSetup:
			return m.updateSetup(msg)
		case viewComment:
			return m.updateComment(msg)
		case viewDetail:
			return m.updateDetail(msg)
		case viewLaunch:
			return m.updateLaunch(msg)
		case viewPrompt:
			return m.updatePrompt(msg)
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
		m.statusMsg = "Setup complete! API key stored in OS keychain."
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
		m.view = viewSetup
		m.apiKeyInput.SetValue("")
		m.teamKeyInput.SetValue("")
		m.setupField = fieldAPIKey
		m.apiKeyInput.Focus()
		return m, textinput.Blink
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
	var b strings.Builder
	fmt.Fprintf(&b, "You're working on %s: %s", issue.Identifier, issue.Title)
	if issue.Description != "" {
		b.WriteString("\n\n")
		b.WriteString(issue.Description)
	}
	if includeComments && len(m.cachedComments) > 0 {
		b.WriteString("\n\n---\nComments:\n")
		for _, c := range m.cachedComments {
			name := c.User.DisplayName
			if name == "" {
				name = c.User.Name
			}
			fmt.Fprintf(&b, "\n@%s:\n%s\n", name, c.Body)
		}
	}
	return b.String()
}

func (m Model) launchWithPromptCmd(issue Issue, prompt string) tea.Cmd {
	return func() tea.Msg {
		wtPath, err := CreateWorktree(issue.Identifier, m.cfg)
		if err != nil {
			return worktreeCreatedMsg{err: err, identifier: issue.Identifier}
		}
		return launchReadyMsg{issue: issue, wtPath: wtPath, prompt: prompt}
	}
}

func (m *Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

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
	parts = append(parts, "c:claude w:wt m:comment enter:detail x:close tab:filter")
	return strings.Join(parts, " | ")
}

// --- View ---

func (m Model) View() string {
	switch m.view {
	case viewSetup:
		return m.viewSetup()
	case viewComment:
		return m.viewComment()
	case viewDetail:
		return m.viewDetail()
	case viewLaunch:
		return m.viewLaunch()
	case viewPrompt:
		return m.viewPrompt()
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

	var b strings.Builder
	b.WriteString(issueIdentStyle.Render(issue.Identifier))
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")).Render(issue.State.Name))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Width(width).Render(issue.Title))
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
		b.WriteString(wrap(strings.Join(labels, ", ")) + "\n")
	}

	if issue.BranchName != "" {
		b.WriteString(commentDimStyle.Render("Branch: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Width(width).Render(issue.BranchName) + "\n")
	}

	if issue.URL != "" {
		b.WriteString(commentDimStyle.Render("URL: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Width(width).Render(issue.URL) + "\n")
	}

	if issue.Description != "" {
		b.WriteString("\n")
		b.WriteString(commentDimStyle.Render("Description:"))
		b.WriteString("\n")
		b.WriteString(wrap(issue.Description))
		b.WriteString("\n")
	}

	if m.cachedCommentID == issue.ID && len(m.cachedComments) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Render("Comments"))
		b.WriteString("\n")
		for _, c := range m.cachedComments {
			name := c.User.DisplayName
			if name == "" {
				name = c.User.Name
			}
			b.WriteString(commentDimStyle.Render(fmt.Sprintf("\n%s:", name)))
			b.WriteString("\n")
			b.WriteString(wrap(c.Body))
			b.WriteString("\n")
		}
	}

	return b.String()
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

func (m Model) viewSetup() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("linear-worktree setup"))
	b.WriteString("\n\n")
	b.WriteString("Linear API Key:\n")
	b.WriteString(m.apiKeyInput.View())
	b.WriteString("\n\n")
	b.WriteString("Team Key (e.g. MYTEAM):\n")
	b.WriteString(m.teamKeyInput.View())
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render(
		"API key will be stored in the OS keychain.\n[Tab] next field  [Enter] save  [Esc] cancel",
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
