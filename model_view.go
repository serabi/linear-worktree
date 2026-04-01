package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

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
	case viewLinkPicker:
		return m.viewPicker("Open Link", m.linkPickerForm)
	case viewFilterPicker:
		return m.viewPicker("Filter Issues", m.filterForm)
	case viewSearch:
		return m.viewSearchInput()
	default:
		return m.viewList()
	}
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

	listHint := ""
	if len(m.issues) == 0 && m.filter == FilterAssigned {
		listHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EAB308")).
			Padding(1, 2).
			Render(fmt.Sprintf("No issues assigned to you in %s. Press tab or f to see all issues.", m.cfg.TeamKey))
	} else if len(m.issues) == 0 {
		listHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Padding(1, 2).
			Render("No issues found.")
	}

	status := statusBarStyle.Render(m.statusMsg)
	hint := statusBarStyle.Render("? help")
	base := appStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, slotBar, teamBar, listHint, content, status, hint),
	)

	if m.showHelp {
		m.help.ShowAll = true
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

	loadingHint := ""
	if m.detailIssue != nil && m.cachedCommentID != m.detailIssue.ID {
		loadingHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7C3AED")).
			Padding(0, 1).
			Render(m.spinner.View() + " Loading comments...")
	}

	body := m.detailViewport.View()
	scrollPct := fmt.Sprintf("%3.f%%", m.detailViewport.ScrollPercent()*100)
	status := statusBarStyle.Render(
		fmt.Sprintf("%s | d/esc:back  j/k:scroll  m:comment  t:transition  g:open  q:quit", scrollPct),
	)

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, loadingHint, body, status))
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
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, m.list.View(), commentBar, status))
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
