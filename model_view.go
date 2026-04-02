package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	var base string
	switch m.view {
	case viewSettings:
		base = m.viewSettings()
	case viewComment:
		base = m.viewComment()
	case viewDetail:
		base = m.viewDetail()
	case viewLaunch:
		base = m.viewLaunch()
	case viewPrompt:
		base = m.viewPrompt()
	case viewProjectPicker:
		base = m.viewPicker("Select Project", m.projectForm)
	case viewLabelPicker:
		base = m.viewPicker("Filter by Label", m.labelForm)
	case viewStatePicker:
		base = m.viewPicker("Transition State", m.stateForm)
	case viewLinkList:
		base = m.viewLinkList()
	case viewFilterPicker:
		base = m.viewPicker("Filter Issues", m.filterForm)
	case viewSortPicker:
		base = m.viewPicker("Sort Issues", m.sortForm)
	case viewSearch:
		base = m.viewSearchInput()
	default:
		base = m.viewList()
	}

	if m.confirm != nil {
		yKey := lipgloss.NewStyle().Bold(true).Foreground(greenColor).Render("y")
		nKey := lipgloss.NewStyle().Bold(true).Foreground(redColor).Render("n")
		dialog := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 3).
			Render(
				titleStyle.Render(m.confirm.title) + "\n\n" +
					m.confirm.message + "\n\n" +
					yKey + " yes  " + nKey + " no",
			)
		base = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	v := tea.NewView(base)
	v.AltScreen = true
	return v
}

func (m Model) renderTeamTabBar() string {
	if len(m.cfg.Teams) <= 1 {
		return ""
	}
	var tabs []string
	for i, t := range m.cfg.Teams {
		label := fmt.Sprintf("[%d] %s", i+1, t.Key)
		if t.Key == m.cfg.TeamKey {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) viewList() string {
	slotBar := m.renderSlotBar()
	teamBar := m.renderTeamTabBar()
	if teamBar != "" {
		teamBar += "\n"
	}
	content := m.list.View()

	if m.loading {
		loadingBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 3).
			Render(m.spinner.View() + "  Loading issues...")
		overlay := lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, loadingBox)
		return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, slotBar, teamBar, overlay))
	}

	if len(m.issues) == 0 {
		listH := m.list.Height()
		var hint string
		if m.filter == FilterAssigned {
			scope := m.cfg.TeamKey
			if m.projectName != "" {
				scope += " > " + m.projectName
			}
			if m.labelName != "" {
				scope += " > " + m.labelName
			}
			hint = lipgloss.NewStyle().
				Foreground(yellowColor).
				Render(fmt.Sprintf("No issues assigned to you in %s.\nPress tab or f to see all issues.", scope))
		} else {
			hint = lipgloss.NewStyle().
				Foreground(dimColor).
				Render("No issues found.")
		}
		content = lipgloss.Place(m.width-2, listH, lipgloss.Center, lipgloss.Center, hint)
	}

	status := statusBarStyle.Render(m.statusMsg)
	legend := statusBarStyle.Render(renderLegendCompact())
	var row1, row2 string
	if len(m.cfg.Teams) > 1 {
		row1 = "enter:detail  c:claude  1-9:team  p:project  L:labels"
		row2 = "f:filter  o:sort  s:settings  ?:help"
	} else {
		row1 = "enter:detail  c:claude  p:project  L:labels"
		row2 = "f:filter  o:sort  s:settings(+teams)  ?:help"
	}
	shortcutText := row1 + "  " + row2
	if lipgloss.Width(shortcutText)+2 > m.width {
		shortcutText = row1 + "\n" + row2
	}
	shortcuts := statusBarStyle.Render(shortcutText)
	base := appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, slotBar, teamBar, content, status, legend, shortcuts),
	)

	if m.showHelp {
		m.help.ShowAll = true
		helpWidth := m.width - 10
		if helpWidth < 40 {
			helpWidth = 40
		}
		m.help.SetWidth(helpWidth)
		helpContent := m.help.View(m.keys)
		legend := renderLegend()
		helpBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Render(titleStyle.Render("Keybindings") + "\n\n" + helpContent + "\n\n" + legend + "\n\n" + statusBarStyle.Render("Press ? to close"))
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpBox)
	}

	return base
}

func (m Model) viewDetail() string {
	identifier := ""
	if m.detailIssue != nil {
		identifier = m.detailIssue.Identifier
	}

	header := titleStyle.Render(fmt.Sprintf("Issue: %s", identifier))

	backLabel := "d:back"
	if len(m.detailHistory) > 0 {
		backLabel = "d:prev issue"
	}

	sortLabel := "o:oldest"
	if !m.commentSortAsc {
		sortLabel = "o:newest"
	}
	detailKeys := func(prefix string) string {
		line := fmt.Sprintf("%s%s  j/k:scroll  l:links  m:comment  r:refresh  %s  s:status  g:open  q:quit", prefix, backLabel, sortLabel)
		if lipgloss.Width(line)+2 <= m.width {
			return statusBarStyle.Render(line)
		}
		row1 := fmt.Sprintf("%s%s  j/k:scroll  l:links  m:comment  r:refresh  %s", prefix, backLabel, sortLabel)
		row2 := "s:status  g:open  q:quit"
		return statusBarStyle.Render(row1 + "\n" + row2)
	}

	status := detailKeys("")

	if m.loading && m.detailIssue != nil {
		loadingBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 3).
			Render(m.spinner.View() + "  " + m.loadingLabel)
		overlay := lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, loadingBox)
		return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, overlay, status))
	}

	body := m.detailViewport.View()
	scrollPct := fmt.Sprintf("%3.f%%", m.detailViewport.ScrollPercent()*100)
	status = detailKeys(scrollPct + " | ")

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
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
			continue
		}

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
		parts[i] = style.Render(
			fmt.Sprintf("[%d] %s %s (%s)", i+1, slot.Status.String(), slot.Issue.Identifier, slot.Status.Label()),
		)
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, "  "))
}

func (m Model) viewLinkList() string {
	status := statusBarStyle.Render("[Enter] open  [Esc] back")
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, m.linkList.View(), status))
}

func (m Model) viewLaunch() string {
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, m.launchList.View()))
}

func (m Model) viewPrompt() string {
	identifier := ""
	if m.launchIssue != nil {
		identifier = m.launchIssue.Identifier
	}
	header := titleStyle.Render(fmt.Sprintf("Prompt for %s", identifier))
	status := statusBarStyle.Render("ctrl+s:launch  esc:back")
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, m.promptArea.View(), status))
}

func (m Model) viewComment() string {
	identifier := ""
	if m.commentIssue != nil {
		identifier = m.commentIssue.Identifier
	}

	commentBar := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("#7C3AED")).
		Render(fmt.Sprintf("💬 Comment on %s: ", identifier)) + m.commentInput.View()

	status := statusBarStyle.Render("[Enter] post  [Esc] cancel")
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, m.detailViewport.View(), commentBar, status))
}

func (m Model) viewSettings() string {
	if m.settingsTabs[0] == nil {
		return ""
	}
	header := titleStyle.Render("Settings")
	tabBar := m.renderSettingsTabBar()
	body := m.activeSettingsForm().View()
	helpText := "Tab: next field  Enter: save  1/2/3: switch tab"
	if m.settingsFirstRun {
		helpText += "  (complete setup to continue)"
	} else {
		helpText += "  Esc: cancel"
	}
	help := statusBarStyle.Render(helpText)
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, tabBar, "", body, "", help))
}

func (m Model) viewSearchInput() string {
	searchBar := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("#7C3AED")).
		Render("Search: ") + m.searchInput.View()

	status := statusBarStyle.Render("[Enter] search  [Esc] cancel")
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, m.list.View(), searchBar, status))
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

func renderLegendCompact() string {
	return statusIcon("backlog") + " Backlog " +
		statusIcon("unstarted") + " Todo " +
		statusIcon("started") + " In Progress " +
		statusIcon("completed") + " Done  " +
		priorityIcon(1) + " Urgent " +
		priorityIcon(2) + " High " +
		priorityIcon(3) + " Med " +
		priorityIcon(4) + " Low"
}

func renderLegend() string {
	heading := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))

	statusLegend := heading.Render("Status") + "\n" +
		statusIcon("backlog") + " Backlog  " +
		statusIcon("unstarted") + " Todo  " +
		statusIcon("started") + " In Progress  " +
		statusIcon("completed") + " Done  " +
		statusIcon("cancelled") + " Cancelled"

	priorityLegend := heading.Render("Priority") + "\n" +
		priorityIcon(1) + " Urgent  " +
		priorityIcon(2) + " High  " +
		priorityIcon(3) + " Medium  " +
		priorityIcon(4) + " Low"

	return statusLegend + "\n\n" + priorityLegend
}
