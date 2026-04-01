package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
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
	case viewStatePicker:
		base = m.viewPicker("Transition State", m.stateForm)
	case viewLinkPicker:
		base = m.viewPicker("Open Link", m.linkPickerForm)
	case viewFilterPicker:
		base = m.viewPicker("Filter Issues", m.filterForm)
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
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	return base
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
				scope = m.cfg.TeamKey + " > " + m.projectName
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
	shortcutText := "enter:detail  c:claude  p:project  f:filter  s:settings  ?:help"
	if len(m.cfg.Teams) > 1 {
		shortcutText = "enter:detail  c:claude  1-9:team  p:project  f:filter  s:settings  ?:help"
	} else {
		shortcutText = "enter:detail  c:claude  p:project  f:filter  s:settings(+teams)  ?:help"
	}
	shortcuts := statusBarStyle.Render(shortcutText)
	base := appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, slotBar, teamBar, content, status, shortcuts),
	)

	if m.showHelp {
		m.help.ShowAll = true
		helpWidth := m.width - 10
		if helpWidth < 40 {
			helpWidth = 40
		}
		m.help.Width = helpWidth
		helpContent := m.help.View(m.keys)
		helpBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Render(titleStyle.Render("Keybindings") + "\n\n" + helpContent + "\n\n" + statusBarStyle.Render("Press ? to close"))
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

	status := statusBarStyle.Render(
		"d/esc:back  j/k:scroll  m:comment  t:transition  g:open  q:quit",
	)

	if m.loading && m.detailIssue != nil && m.cachedCommentID != m.detailIssue.ID {
		loadingBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 3).
			Render(m.spinner.View() + "  Loading comments...")
		overlay := lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, loadingBox)
		return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, overlay, status))
	}

	body := m.detailViewport.View()
	scrollPct := fmt.Sprintf("%3.f%%", m.detailViewport.ScrollPercent()*100)
	status = statusBarStyle.Render(
		fmt.Sprintf("%s | d/esc:back  j/k:scroll  m:comment  t:transition  g:open  q:quit", scrollPct),
	)

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
	help := statusBarStyle.Render("Tab: next field  Enter: save  1/2/3: switch tab  Esc: cancel")
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
