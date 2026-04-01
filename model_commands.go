package main

import (
	"fmt"
	osexec "os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) fetchIssues() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		if m.projectFilter != nil {
			if *m.projectFilter == "none" {
				issues, _, err := client.GetIssuesWithNoProject(m.cfg.TeamID, m.filter, "")
				return issuesLoadedMsg{issues: issues, err: err}
			}
			issues, _, err := client.GetIssuesByProject(m.cfg.TeamID, *m.projectFilter, "")
			return issuesLoadedMsg{issues: issues, err: err}
		}
		issues, _, err := client.GetIssues(m.cfg.TeamID, m.filter, "")
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

func (m Model) fetchViewer() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		viewer, err := client.GetViewer()
		return viewerLoadedMsg{viewer: viewer, err: err}
	}
}

func (m Model) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		projects, err := client.GetProjects(m.cfg.TeamID)
		return projectsLoadedMsg{projects: projects, err: err}
	}
}

func (m Model) fetchWorkflowStates() tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		states, err := client.GetWorkflowStates(m.cfg.TeamID)
		return statesLoadedMsg{states: states, err: err}
	}
}

func (m Model) assignToMeCmd(issueID, assigneeID, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.UpdateIssueAssignee(issueID, assigneeID)
		return issueAssignedMsg{identifier: identifier, err: err}
	}
}

func (m Model) unassignCmd(issueID, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.UnassignIssue(issueID)
		return issueUnassignedMsg{identifier: identifier, err: err}
	}
}

func (m Model) changeStateCmd(issueID, stateID, identifier string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		err := client.UpdateIssueState(issueID, stateID)
		return issueStateChangedMsg{identifier: identifier, err: err}
	}
}

func (m Model) detectBranchIssue() tea.Cmd {
	return func() tea.Msg {
		out, err := osexec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err != nil {
			return branchIssueFoundMsg{}
		}
		branch := strings.TrimSpace(string(out))
		if !strings.HasPrefix(branch, m.cfg.BranchPrefix) {
			return branchIssueFoundMsg{}
		}
		client := NewLinearClient(m.cfg.LinearAPIKey)
		issue, _ := client.SearchIssueByBranch(branch)
		return branchIssueFoundMsg{issue: issue}
	}
}

func (m Model) resolveTeamCmd(apiKey string, teamKeys []string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(apiKey)
		var teams []TeamEntry
		for _, k := range teamKeys {
			team, err := client.GetTeamByKey(k)
			if err != nil {
				return teamsLoadedMsg{err: fmt.Errorf("team %q: %w", k, err)}
			}
			teams = append(teams, TeamEntry{ID: team.ID, Key: team.Key})
		}

		cfg := m.cfg
		cfg.LinearAPIKey = apiKey
		cfg.Teams = teams
		cfg.TeamID = teams[0].ID
		cfg.TeamKey = teams[0].Key
		if err := SaveConfig(cfg); err != nil {
			return teamsLoadedMsg{err: err}
		}
		return setupCompleteMsg{cfg: cfg}
	}
}

func (m Model) startStatusPoll() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return statusPollMsg{}
	})
}

func (m Model) launchWithPromptCmd(issue Issue, prompt string) tea.Cmd {
	return func() tea.Msg {
		wtPath, err := CreateWorktree(issue.Identifier, m.cfg)
		if err != nil {
			return worktreeCreatedMsg{err: err, identifier: issue.Identifier}
		}
		if err := RunPostCreateHook(wtPath, m.cfg); err != nil {
			debugLog.Printf("post-create hook failed: %v", err)
		}
		return launchReadyMsg{issue: issue, wtPath: wtPath, prompt: prompt}
	}
}

func (m Model) searchIssuesCmd(term string) tea.Cmd {
	return func() tea.Msg {
		client := NewLinearClient(m.cfg.LinearAPIKey)
		issues, _, err := client.SearchIssues(term, m.cfg.TeamID, 50, "")
		return searchResultsMsg{issues: issues, err: err}
	}
}

func (m Model) switchTeamCmd(team TeamEntry) tea.Cmd {
	return func() tea.Msg {
		cfg := m.cfg
		cfg.TeamID = team.ID
		cfg.TeamKey = team.Key
		if err := SaveConfig(cfg); err != nil {
			return teamSwitchedMsg{err: err}
		}
		return teamSwitchedMsg{cfg: cfg}
	}
}
