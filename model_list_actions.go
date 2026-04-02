package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) cycleFilter() (tea.Model, tea.Cmd) {
	m.filter = m.filter.Next()
	m.updateListTitle()
	m.loading = true
	m.loadingLabel = "Loading..."
	return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)
}

func (m *Model) refreshList() (tea.Model, tea.Cmd) {
	m.loading = true
	m.loadingLabel = "Refreshing..."
	return m, tea.Batch(m.fetchIssues(), m.fetchWorktrees(), m.spinner.Tick)
}

func (m *Model) beginLaunchFromSelection() (tea.Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue == nil {
		m.statusMsg = "No issue selected"
		return m, nil
	}

	m.launchIssue = issue
	m.view = viewLaunch
	items := m.buildLaunchOptions(issue)
	m.launchList.Title = fmt.Sprintf("Launch Claude for %s", issue.Identifier)
	m.launchList.SetItems(items)
	m.launchList.SetSize(m.width-4, len(items)*3+4)

	if issue.ID != m.cachedCommentID {
		return m, m.fetchCommentsCmd(issue.ID)
	}
	return m, nil
}

func (m Model) buildLaunchOptions(issue *Issue) []list.Item {
	items := []list.Item{
		launchOption{"prompt", "Launch with prompt", "Edit prompt before launching Claude", -1},
		launchOption{"blank", "Launch blank session", "Open Claude with no initial message", -1},
	}

	if m.paneManager != nil {
		for _, slot := range m.paneManager.Slots() {
			if slot != nil && slot.Issue.Identifier == issue.Identifier {
				items = append([]list.Item{
					launchOption{
						"resume",
						"Resume existing session",
						fmt.Sprintf("Focus slot %d (%s)", slot.Index+1, slot.Status.Label()),
						slot.Index,
					},
				}, items...)
				break
			}
		}
	}

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

	return items
}

func (m *Model) createSelectedWorktree() (tea.Model, tea.Cmd) {
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
		hookErr := RunPostCreateHook(wtPath, m.cfg)
		return worktreeCreatedMsg{path: wtPath, identifier: issue.Identifier, hookErr: hookErr}
	}
}

func (m *Model) closeSelectedSlot() (tea.Model, tea.Cmd) {
	if m.paneManager == nil {
		m.statusMsg = "No cmux panes to close"
		return m, nil
	}

	issue := m.selectedIssue()
	if issue == nil {
		return m, nil
	}

	for i, slot := range m.paneManager.Slots() {
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
}

func (m *Model) showSelectedIssueDetail() (tea.Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue == nil {
		m.statusMsg = "No issue selected"
		return m, nil
	}

	m.view = viewDetail
	m.detailIssue = issue
	m.detailHistory = nil
	m.detailViewport.SetWidth(m.width - 6)
	m.detailViewport.SetHeight(m.height - 6)
	m.loading = true
	m.loadingLabel = "Loading..."
	cmds := []tea.Cmd{m.buildDetailContentCmd(issue), m.spinner.Tick}
	if issue.ID != m.cachedCommentID {
		cmds = append(cmds, m.fetchCommentsCmd(issue.ID))
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) beginComment(issue *Issue) (tea.Model, tea.Cmd) {
	m.view = viewComment
	m.commentIssue = issue
	m.commentInput.SetValue("")
	m.commentInput.Focus()
	m.statusMsg = fmt.Sprintf("Comment on %s (Enter to post, Esc to cancel)", issue.Identifier)
	return m, textinput.Blink
}

func (m *Model) openSelectedIssue() (tea.Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue != nil && issue.URL != "" {
		openBrowser(issue.URL)
	}
	return m, nil
}

func (m *Model) switchTeamFromKey(key string) (tea.Model, tea.Cmd) {
	if len(m.cfg.Teams) <= 1 || key == "" {
		return m, nil
	}

	idx := int(key[0] - '1')
	if idx < len(m.cfg.Teams) && m.cfg.Teams[idx].Key != m.cfg.TeamKey {
		return m, m.switchTeamCmd(m.cfg.Teams[idx])
	}
	return m, nil
}

func (m *Model) beginSearchMode() (tea.Model, tea.Cmd) {
	m.view = viewSearch
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	return m, textinput.Blink
}

func (m *Model) assignSelectedIssueToViewer() (tea.Model, tea.Cmd) {
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

func (m *Model) unassignSelectedIssue() (tea.Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue == nil {
		m.statusMsg = "No issue selected"
		return m, nil
	}
	if issue.Assignee == nil {
		m.statusMsg = fmt.Sprintf("%s is already unassigned", issue.Identifier)
		return m, nil
	}

	m.statusMsg = fmt.Sprintf("Unassigning %s...", issue.Identifier)
	return m, m.unassignCmd(issue.ID, issue.Identifier)
}

func (m *Model) openSelectedIssueLinks() (tea.Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue == nil {
		m.statusMsg = "No issue selected"
		return m, nil
	}

	urls := extractURLs(issue.Description)
	if len(urls) == 0 {
		m.statusMsg = "No links found in description"
		return m, nil
	}
	if len(urls) == 1 {
		openBrowser(urls[0])
		return m, nil
	}
	items := make([]list.Item, len(urls))
	for i, u := range urls {
		items[i] = linkItem{label: truncateURL(u, 60), value: u}
	}
	m.linkReturnToView = viewList
	m.showLinkList(items, "Open link")
	return m, nil
}

func (m *Model) updateListCursor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	prevIndex := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if m.list.Index() == prevIndex {
		return m, cmd
	}

	m.prefetchSeq++
	seq := m.prefetchSeq
	prefetchCmd := tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return prefetchTickMsg{seq: seq}
	})
	return m, tea.Batch(cmd, prefetchCmd)
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
