package main

import (
	"fmt"
	"net/url"
	osexec "os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

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
		options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", s.Name, s.Type), s.ID))
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

func (m *Model) handleStateSelected() (tea.Model, tea.Cmd) {
	selected := m.stateForm.GetString("state")
	issue := m.stateIssue
	m.stateForm = nil
	m.stateIssue = nil
	m.view = viewList

	if selected == "" || issue == nil {
		return m, nil
	}

	// Find the state name for the confirmation message.
	stateName := selected
	for _, s := range m.workflowStates {
		if s.ID == selected {
			stateName = s.Name
			break
		}
	}

	m.confirm = &confirmDialog{
		action:  confirmStateChange,
		title:   "Change State?",
		message: fmt.Sprintf("Move %s to %q?", issue.Identifier, stateName),
		onYes: func(m *Model) (tea.Model, tea.Cmd) {
			m.statusMsg = fmt.Sprintf("Updating state for %s...", issue.Identifier)
			return m, m.changeStateCmd(issue.ID, selected, issue.Identifier)
		},
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

func (m *Model) showLinkList(items []list.Item, title string) {
	m.linkList.Title = title
	m.linkList.SetItems(items)
	m.linkList.SetSize(m.width-4, m.height-4)
	m.linkList.Select(0)
	m.view = viewLinkList
}

func (m *Model) showDetailLinks() (tea.Model, tea.Cmd) {
	issue := m.detailIssue
	var items []list.Item

	if issue.Parent != nil {
		items = append(items, linkItem{
			label:   fmt.Sprintf("Parent: %s  %s", issue.Parent.Identifier, issue.Parent.Title),
			value:   issue.Parent.ID,
			isIssue: true,
		})
	}

	for _, child := range issue.Children.Nodes {
		icon := statusIcon(child.State.Type)
		items = append(items, linkItem{
			label:   fmt.Sprintf("Sub-issue: %s %s  %s", icon, child.Identifier, child.Title),
			value:   child.ID,
			isIssue: true,
		})
	}

	for _, rel := range issue.Relations.Nodes {
		prefix := rel.Type
		switch rel.Type {
		case "blocks":
			prefix = "Blocking"
		case "is blocked by", "blocked":
			prefix = "Blocked by"
		case "related":
			prefix = "Related"
		case "duplicate":
			prefix = "Duplicate"
		}
		items = append(items, linkItem{
			label:   fmt.Sprintf("%s: %s  %s", prefix, rel.RelatedIssue.Identifier, rel.RelatedIssue.Title),
			value:   rel.RelatedIssue.ID,
			isIssue: true,
		})
	}

	for _, u := range extractURLs(issue.Description) {
		items = append(items, linkItem{
			label: truncateURL(u, 60),
			value: u,
		})
	}

	if len(items) == 0 {
		m.statusMsg = "No links or related issues"
		return m, nil
	}

	m.linkReturnToView = viewDetail
	m.showLinkList(items, "Navigate")
	return m, nil
}

func (m *Model) updateLinkList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = m.linkReturnToView
		return m, nil
	case "enter":
		item := m.linkList.SelectedItem()
		if item == nil {
			return m, nil
		}
		li := item.(linkItem)
		if li.isIssue {
			if m.detailIssue != nil {
				m.detailHistory = append(m.detailHistory, m.detailIssue)
			}
			m.loading = true
			m.loadingLabel = "Loading issue..."
			m.view = viewDetail
			return m, tea.Batch(m.navigateToIssueCmd(li.value), m.spinner.Tick)
		}
		openBrowser(li.value)
		m.view = m.linkReturnToView
		return m, nil
	}

	var cmd tea.Cmd
	m.linkList, cmd = m.linkList.Update(msg)
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

func truncateURL(raw string, maxLen int) string {
	if maxLen <= 0 {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if len(raw) > maxLen {
			return raw[:maxLen-1] + "…"
		}
		return raw
	}
	label := u.Host + u.Path
	if u.Fragment != "" {
		label += "#" + u.Fragment
	}
	if len(label) > maxLen {
		label = label[:maxLen-1] + "…"
	}
	return label
}

var urlPattern = regexp.MustCompile(`https?://[^\s)>\]]+`)

func extractURLs(text string) []string {
	seen := make(map[string]bool)
	var urls []string
	for _, u := range urlPattern.FindAllString(text, -1) {
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	return urls
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
