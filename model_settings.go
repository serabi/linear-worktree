package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) activeSettingsForm() *huh.Form {
	return m.settingsTabs[m.settingsActiveTab]
}

func (m *Model) initSettingsForm() {
	draft := &settingsDraft{
		apiKey:     m.cfg.LinearAPIKey,
		wtBase:     m.cfg.WorktreeBase,
		copyFiles:  strings.Join(m.cfg.CopyFiles, ", "),
		copyDirs:   strings.Join(m.cfg.CopyDirs, ", "),
		claudeCmd:  m.cfg.ClaudeCommand,
		claudeArgs: m.cfg.ClaudeArgs,
		branch:     m.cfg.BranchPrefix,
		maxSlots:   m.cfg.MaxSlots,
		hook:       m.cfg.PostCreateHook,
		prompt:     m.cfg.PromptTemplate,
	}
	if len(m.cfg.Teams) > 0 {
		keys := make([]string, len(m.cfg.Teams))
		for i, t := range m.cfg.Teams {
			keys[i] = t.Key
		}
		draft.teamKey = strings.Join(keys, ", ")
	} else {
		draft.teamKey = m.cfg.TeamKey
	}

	m.settingsDraft = draft
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
					Value(&m.settingsDraft.apiKey),
				huh.NewInput().
					Title("Team Keys (comma-separated)").
					Description("Add multiple teams separated by commas. First team is your default. Press 1-9 from the issue list to switch between teams.").
					Placeholder("TSCODE, DHMIG, OTHER").
					Value(&m.settingsDraft.teamKey),
			),
		).WithWidth(w).WithShowHelp(false).WithShowErrors(true)
	case 1:
		return huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Worktree Base Directory").
					Description("Where new git worktrees are created, relative to the repo root. Each issue gets a subdirectory here.").
					Placeholder("../worktrees").
					Value(&m.settingsDraft.wtBase),
				huh.NewInput().
					Title("Files to Copy (comma-separated)").
					Description("Files copied from the main repo into each new worktree. Add multiple separated by commas.").
					Placeholder(".env, .envrc, .tool-versions").
					Value(&m.settingsDraft.copyFiles),
				huh.NewInput().
					Title("Directories to Copy (comma-separated)").
					Description("Directories copied into each new worktree. Add multiple separated by commas.").
					Placeholder(".claude, .config").
					Value(&m.settingsDraft.copyDirs),
				huh.NewInput().
					Title("Branch Prefix").
					Description("Prefix added to git branch names when creating worktrees. Issue TSCODE-123 becomes feature/tscode-123.").
					Placeholder("feature/").
					Value(&m.settingsDraft.branch),
			),
		).WithWidth(w).WithShowHelp(false).WithShowErrors(true)
	default:
		return huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Claude Command").
					Description("The command used to launch Claude Code. Change this if claude is installed at a custom path.").
					Placeholder("claude").
					Value(&m.settingsDraft.claudeCmd).
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
					Value(&m.settingsDraft.claudeArgs),
				huh.NewInput().
					Title("Post-Create Hook").
					Description("Shell command that runs inside the worktree directory after creation. Use for setup tasks like installing dependencies.").
					Placeholder("npm install && direnv allow").
					Value(&m.settingsDraft.hook),
				huh.NewText().
					Title("Prompt Template").
					Description("Custom prompt sent to Claude on launch. Supports Go template variables: {{.Identifier}}, {{.Title}}, {{.Description}}. Leave empty for the default prompt.").
					Value(&m.settingsDraft.prompt),
				huh.NewSelect[int]().
					Title("Max Slots").
					Description("Maximum number of concurrent Claude sessions in the E-layout. Only applies when running inside cmux.").
					Options(
						huh.NewOption("2 slots", 2),
						huh.NewOption("3 slots", 3),
						huh.NewOption("4 slots", 4),
					).
					Value(&m.settingsDraft.maxSlots),
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

func (m *Model) recreatePaneManagerIfNeeded() {
	if m.paneManager == nil || m.paneManager.maxSlots == m.cfg.MaxSlots {
		return
	}
	for i, slot := range m.paneManager.Slots() {
		if slot != nil {
			_ = m.paneManager.CloseSlot(i)
		}
	}
	m.paneManager = NewPaneManager(m.cmuxClient, m.cfg.MaxSlots)
}

func (m *Model) rebuildActiveTab() tea.Cmd {
	w := m.width - 4
	if w < 60 {
		w = 60
	}
	m.settingsTabs[m.settingsActiveTab] = m.buildTab(m.settingsActiveTab, w)
	return m.settingsTabs[m.settingsActiveTab].Init()
}

func (m Model) renderSettingsTabBar() string {
	var tabs []string
	for i, name := range m.settingsTabNames {
		label := fmt.Sprintf("[%d] %s", i+1, name)
		if i == m.settingsActiveTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m *Model) handleSettingsCompleted() (tea.Model, tea.Cmd) {
	debugLog.Printf("handleSettingsCompleted: settingsAPIKey=%q settingsTeamKey=%q", m.settingsDraft.apiKey, m.settingsDraft.teamKey)

	apiKey := strings.TrimSpace(m.settingsDraft.apiKey)
	teamKeys := splitComma(m.settingsDraft.teamKey)

	debugLog.Printf("handleSettingsCompleted: parsed teamKeys=%v", teamKeys)

	if apiKey == "" || len(teamKeys) == 0 {
		m.statusMsg = "API key and at least one team key are required"
		m.settingsActiveTab = 0
		return m, nil
	}

	newCfg := m.cfg
	newCfg.LinearAPIKey = apiKey
	newCfg.WorktreeBase = strings.TrimSpace(m.settingsDraft.wtBase)
	newCfg.CopyFiles = splitComma(m.settingsDraft.copyFiles)
	newCfg.CopyDirs = splitComma(m.settingsDraft.copyDirs)
	newCfg.ClaudeCommand = strings.TrimSpace(m.settingsDraft.claudeCmd)
	newCfg.ClaudeArgs = strings.TrimSpace(m.settingsDraft.claudeArgs)
	newCfg.BranchPrefix = strings.TrimSpace(m.settingsDraft.branch)
	newCfg.MaxSlots = m.settingsDraft.maxSlots
	newCfg.PostCreateHook = strings.TrimSpace(m.settingsDraft.hook)
	newCfg.PromptTemplate = m.settingsDraft.prompt

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

	oldKeys := make([]string, len(m.cfg.Teams))
	for i, t := range m.cfg.Teams {
		oldKeys[i] = t.Key
	}
	debugLog.Printf("settings save: teamKeys=%v oldKeys=%v", teamKeys, oldKeys)
	if strings.Join(teamKeys, ",") != strings.Join(oldKeys, ",") || newCfg.TeamID == "" {
		m.cfg = newCfg
		m.statusMsg = "Resolving teams..."
		return m, m.resolveTeamCmd(apiKey, teamKeys)
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
	m.recreatePaneManagerIfNeeded()
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

func (m *Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsTabs[0] == nil {
		return m, nil
	}

	switch msg.String() {
	case "1":
		m.settingsActiveTab = 0
		return m, nil
	case "2":
		m.settingsActiveTab = 1
		return m, nil
	case "3":
		m.settingsActiveTab = 2
		return m, nil
	case "ctrl+s":
		for _, tab := range m.settingsTabs {
			if tab != nil && tab.State == huh.StateNormal {
				tab.GetFocusedField().Blur()
			}
		}
		return m.handleSettingsCompleted()
	case "esc":
		if m.settingsFirstRun {
			return m, nil
		}
		m.settingsTabs = [3]*huh.Form{}
		m.view = viewList
		return m, nil
	}

	f := m.activeSettingsForm()
	debugLog.Printf("updateSettings key=%q tab=%d teamKey=%q formState=%d", msg.String(), m.settingsActiveTab, m.settingsDraft.teamKey, f.State)
	form, cmd := f.Update(msg)
	if updated, ok := form.(*huh.Form); ok {
		m.settingsTabs[m.settingsActiveTab] = updated
	}
	debugLog.Printf("updateSettings after update: teamKey=%q formState=%d", m.settingsDraft.teamKey, m.activeSettingsForm().State)

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
		if m.settingsActiveTab > 0 {
			initCmd := m.rebuildActiveTab()
			m.settingsActiveTab--
			return m, tea.Batch(cmd, initCmd)
		}
		if !m.settingsFirstRun {
			m.settingsTabs = [3]*huh.Form{}
			m.view = viewList
			return m, nil
		}
		return m, tea.Batch(cmd, m.rebuildActiveTab())
	}

	return m, cmd
}
