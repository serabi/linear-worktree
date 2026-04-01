package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listHeight := msg.Height - 6
		if m.useCmux {
			listHeight--
		}
		if len(m.cfg.Teams) > 1 {
			listHeight -= 2
		}
		m.list.SetSize(msg.Width-2, listHeight)
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
		if m.confirm != nil {
			switch msg.String() {
			case "y":
				c := m.confirm
				m.confirm = nil
				return c.onYes(&m)
			default:
				m.confirm = nil
				return m, nil
			}
		}

		textInputView := m.view == viewSearch || m.view == viewComment || m.view == viewPrompt || m.view == viewSettings
		if msg.String() == "ctrl+c" || (msg.String() == "q" && !textInputView) {
			if m.list.FilterState() == list.Filtering && msg.String() != "ctrl+c" {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				return m, cmd
			}
			m.confirm = &confirmDialog{
				action:  confirmQuit,
				title:   "Quit?",
				message: "Are you sure you want to exit?",
				onYes:   func(m *Model) (tea.Model, tea.Cmd) { return m, tea.Quit },
			}
			return m, nil
		}

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
		case viewLinkList:
			return m.updateLinkList(msg)
		default:
			return m.updateList(msg)
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			if m.view != viewDetail {
				m.statusMsg = m.spinner.View() + " " + m.loadingLabel
			}
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
		if msg.hookErr != nil {
			m.statusMsg = fmt.Sprintf("Worktree created: %s (hook failed: %v)", msg.path, msg.hookErr)
		} else {
			m.statusMsg = fmt.Sprintf("Worktree created: %s", msg.path)
		}
		return m, m.fetchWorktrees()

	case cmuxSlotOpenedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error opening slot: %v", msg.err)
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
			if m.commentIssue != nil {
				return m, m.fetchCommentsCmd(m.commentIssue.ID)
			}
		}
		return m, nil

	case commentsLoadedMsg:
		if m.loadingLabel == "Loading comments..." {
			m.loading = false
		}
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

	case prefetchTickMsg:
		if msg.seq == m.prefetchSeq {
			issue := m.selectedIssue()
			if issue != nil && issue.ID != m.cachedCommentID {
				return m, m.fetchCommentsCmd(issue.ID)
			}
		}
		return m, nil

	case teamSwitchedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Team switch error: %v", msg.err)
			return m, nil
		}
		m.cfg = msg.cfg
		m.flushTeamState()
		m.loading = true
		m.loadingLabel = fmt.Sprintf("Loading %s...", m.cfg.TeamKey)
		return m, tea.Batch(m.fetchIssues(), m.fetchProjects(), m.fetchWorkflowStates(), m.spinner.Tick)

	case setupCompleteMsg:
		m.cfg = msg.cfg
		m.view = viewList
		m.settingsTabs = [3]*huh.Form{}
		m.settingsFirstRun = false
		m.statusMsg = "Settings saved. API key stored in OS keychain."
		m.keys.TeamSwitch.SetEnabled(len(m.cfg.Teams) > 1)
		m.updateListTitle()
		m.recreatePaneManagerIfNeeded()
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

	case issueUnassignedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Unassign error: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Unassigned %s", msg.identifier)
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
		m.statusMsg = fmt.Sprintf("Search: %q (%d results)", m.searchTerm, len(msg.issues))
		return m, nil

	case issueNavigatedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Navigation error: %v", msg.err)
			return m, nil
		}
		m.detailIssue = msg.issue
		contentWidth := m.width - 6
		m.detailViewport.Width = contentWidth
		m.detailViewport.Height = m.height - 6
		m.detailViewport.SetContent(m.buildDetailContent(msg.issue, contentWidth))
		m.detailViewport.GotoTop()
		m.view = viewDetail
		if msg.issue.ID != m.cachedCommentID {
			m.loading = true
			m.loadingLabel = "Loading comments..."
			return m, tea.Batch(m.fetchCommentsCmd(msg.issue.ID), m.spinner.Tick)
		}
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

	if m.view == viewSettings && m.settingsTabs[0] != nil {
		f := m.activeSettingsForm()
		form, cmd := f.Update(msg)
		if updated, ok := form.(*huh.Form); ok {
			m.settingsTabs[m.settingsActiveTab] = updated
		}
		active := m.activeSettingsForm()
		if active.State == huh.StateCompleted {
			for _, tab := range m.settingsTabs {
				if tab != nil && tab.State == huh.StateNormal {
					tab.GetFocusedField().Blur()
				}
			}
			return m.handleSettingsCompleted()
		}
		if active.State == huh.StateAborted {
			return m, tea.Batch(cmd, m.rebuildActiveTab())
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
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		if m.showHelp {
			m.showHelp = false
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		return m.cycleFilter()

	case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
		return m, m.showFilterPicker()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		return m.refreshList()

	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		return m.beginLaunchFromSelection()

	case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
		return m.createSelectedWorktree()

	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		issue := m.selectedIssue()
		if issue == nil {
			return m, nil
		}
		m.confirm = &confirmDialog{
			action:  confirmCloseSlot,
			title:   "Close Slot?",
			message: fmt.Sprintf("Close the Claude session for %s?", issue.Identifier),
			onYes:   func(m *Model) (tea.Model, tea.Cmd) { return m.closeSelectedSlot() },
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("d", "enter"))):
		return m.showSelectedIssueDetail()

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		return m.openSelectedIssue()

	case key.Matches(msg, key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"))):
		return m.switchTeamFromKey(msg.String())

	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		return m, m.buildSettingsForm()

	case key.Matches(msg, key.NewBinding(key.WithKeys("p"))):
		return m, m.showProjectPicker()

	case key.Matches(msg, key.NewBinding(key.WithKeys("?"))):
		m.showHelp = !m.showHelp
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("S"))):
		return m.beginSearchMode()

	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.confirm = &confirmDialog{
			action:  confirmAssign,
			title:   "Assign?",
			message: fmt.Sprintf("Assign %s to you?", issue.Identifier),
			onYes:   func(m *Model) (tea.Model, tea.Cmd) { return m.assignSelectedIssueToViewer() },
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
		issue := m.selectedIssue()
		if issue == nil {
			m.statusMsg = "No issue selected"
			return m, nil
		}
		m.confirm = &confirmDialog{
			action:  confirmUnassign,
			title:   "Unassign?",
			message: fmt.Sprintf("Unassign %s?", issue.Identifier),
			onYes:   func(m *Model) (tea.Model, tea.Cmd) { return m.unassignSelectedIssue() },
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
		return m.openSelectedIssueLinks()
	}

	return m.updateListCursor(msg)
}

func (m *Model) updateComment(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewDetail
		m.statusMsg = m.buildStatusLine()
		return m, nil

	case "enter":
		body := strings.TrimSpace(m.commentInput.Value())
		if body == "" {
			m.statusMsg = "Empty comment, cancelled"
			m.view = viewDetail
			return m, nil
		}
		issue := m.commentIssue
		m.confirm = &confirmDialog{
			action:  confirmPostComment,
			title:   "Post Comment?",
			message: fmt.Sprintf("Post comment on %s?", issue.Identifier),
			onYes: func(m *Model) (tea.Model, tea.Cmd) {
				m.view = viewDetail
				m.statusMsg = fmt.Sprintf("Posting comment on %s...", issue.Identifier)
				return m, m.postCommentCmd(issue.ID, body, issue.Identifier)
			},
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.commentInput, cmd = m.commentInput.Update(msg)
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "d":
		if len(m.detailHistory) > 0 {
			prev := m.detailHistory[len(m.detailHistory)-1]
			m.detailHistory = m.detailHistory[:len(m.detailHistory)-1]
			m.detailIssue = prev
			contentWidth := m.width - 6
			m.detailViewport.SetContent(m.buildDetailContent(prev, contentWidth))
			m.detailViewport.GotoTop()
			if prev.ID != m.cachedCommentID {
				m.loading = true
				m.loadingLabel = "Loading comments..."
				return m, tea.Batch(m.fetchCommentsCmd(prev.ID), m.spinner.Tick)
			}
			m.loading = false
			return m, nil
		}
		m.view = viewList
		m.detailIssue = nil
		m.loading = false
		return m, nil
	case "m":
		if m.detailIssue != nil {
			return m.beginComment(m.detailIssue)
		}
		return m, nil
	case "g":
		if m.detailIssue != nil && m.detailIssue.URL != "" {
			openBrowser(m.detailIssue.URL)
		}
		return m, nil
	case "l":
		if m.detailIssue != nil {
			return m.showDetailLinks()
		}
		return m, nil
	case "t":
		if m.detailIssue != nil {
			m.stateIssue = m.detailIssue
			if len(m.workflowStates) > 0 {
				return m, m.showStatePicker()
			}
			return m, m.fetchWorkflowStates()
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
