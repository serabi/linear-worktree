package main

import (
	"fmt"
	osexec "os/exec"
	"regexp"
	"strings"

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

func (m *Model) showSortPicker() tea.Cmd {
	options := []huh.Option[string]{
		huh.NewOption("Recently updated", "updated"),
		huh.NewOption("Recently created", "created"),
		huh.NewOption("Priority", "priority"),
	}

	m.sortForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("sort").
				Title("Sort issues").
				Options(options...),
		),
	).WithWidth(50).WithShowHelp(false).WithShowErrors(false)
	m.view = viewSortPicker
	return m.sortForm.Init()
}

func (m *Model) handleSortSelected() (tea.Model, tea.Cmd) {
	selected := m.sortForm.GetString("sort")
	m.sortForm = nil
	m.view = viewList

	switch selected {
	case "updated":
		m.sortMode = SortUpdatedAt
	case "created":
		m.sortMode = SortCreatedAt
	case "priority":
		m.sortMode = SortPriority
	default:
		return m, nil
	}

	m.updateListTitle()
	m.loading = true
	m.loadingLabel = "Loading..."
	return m, tea.Batch(m.fetchIssues(), m.spinner.Tick)
}

func (m *Model) updateSortPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.sortForm = nil
		return m, nil
	}

	form, cmd := m.sortForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.sortForm = f
	}
	if m.sortForm.State == huh.StateCompleted {
		return m.handleSortSelected()
	}
	return m, cmd
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

func (m *Model) showLinkPicker(urls []string) tea.Cmd {
	options := make([]huh.Option[string], len(urls))
	for i, u := range urls {
		options[i] = huh.NewOption(u, u)
	}

	m.linkPickerURLs = urls
	m.linkSelected = ""
	m.linkPickerForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Open link").
				Options(options...).
				Value(&m.linkSelected),
		),
	).WithWidth(70).WithShowHelp(false).WithShowErrors(false)
	m.view = viewLinkPicker
	return m.linkPickerForm.Init()
}

func (m *Model) updateLinkPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.linkPickerForm = nil
		return m, nil
	}

	form, cmd := m.linkPickerForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.linkPickerForm = f
	}
	if m.linkPickerForm.State == huh.StateCompleted {
		selected := m.linkSelected
		m.linkPickerForm = nil
		m.view = viewList
		if selected != "" {
			openBrowser(selected)
		}
		return m, nil
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
