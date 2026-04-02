package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type issueItem struct {
	issue       Issue
	hasWorktree bool
	slotIdx     int
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

func (i issueItem) Description() string {
	var parts []string
	if i.issue.Assignee != nil {
		name := i.issue.Assignee.DisplayName
		if name == "" {
			name = i.issue.Assignee.Name
		}
		if idx := strings.IndexByte(name, ' '); idx > 0 {
			name = name[:idx]
		}
		parts = append(parts, name)
	} else {
		parts = append(parts, commentDimStyle.Render("unassigned"))
	}

	if i.issue.Project != nil {
		parts = append(parts, i.issue.Project.Name)
	}

	if i.issue.DueDate != nil {
		if t, err := time.Parse("2006-01-02", *i.issue.DueDate); err == nil {
			days := int(time.Until(t).Hours() / 24)
			switch {
			case days < 0:
				parts = append(parts, fmt.Sprintf("OVERDUE %dd", -days))
			case days <= 3:
				parts = append(parts, fmt.Sprintf("Due in %dd", days))
			}
		}
	}

	if i.issue.SLABreachesAt != nil {
		if t, err := time.Parse(time.RFC3339, *i.issue.SLABreachesAt); err == nil {
			d := time.Until(t)
			var slaStr string
			switch {
			case d < 0:
				slaStr = lipgloss.NewStyle().Foreground(redColor).Render("SLA BREACHED")
			case d <= 24*time.Hour:
				slaStr = lipgloss.NewStyle().Foreground(redColor).Render(fmt.Sprintf("SLA %dh", int(d.Hours())))
			case d <= 3*24*time.Hour:
				slaStr = lipgloss.NewStyle().Foreground(orangeColor).Render(fmt.Sprintf("SLA %dd", int(d.Hours()/24)))
			default:
				slaStr = commentDimStyle.Render(fmt.Sprintf("SLA %dd", int(d.Hours()/24)))
			}
			parts = append(parts, slaStr)
		}
	}

	if n := len(i.issue.Children.Nodes); n > 0 {
		done := 0
		for _, c := range i.issue.Children.Nodes {
			if c.State.Type == "completed" {
				done++
			}
		}
		parts = append(parts, fmt.Sprintf("[%d/%d]", done, n))
	}

	if labels := i.issue.Labels.Nodes; len(labels) > 0 {
		maxLabels := 2
		if len(labels) < maxLabels {
			maxLabels = len(labels)
		}
		for _, l := range labels[:maxLabels] {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color))
			parts = append(parts, style.Render(l.Name))
		}
		if remaining := len(labels) - maxLabels; remaining > 0 {
			parts = append(parts, commentDimStyle.Render(fmt.Sprintf("+%d", remaining)))
		}
	}

	return strings.Join(parts, " | ")
}

func (i issueItem) FilterValue() string {
	return i.issue.Identifier + " " + i.issue.Title
}

type launchOption struct {
	action    string
	title     string
	desc      string
	slotIndex int
}

func (l launchOption) Title() string       { return l.title }
func (l launchOption) Description() string { return l.desc }
func (l launchOption) FilterValue() string { return l.title }

type linkItem struct {
	label   string
	value   string
	isIssue bool
}

func (l linkItem) Title() string       { return l.label }
func (l linkItem) Description() string { return "" }
func (l linkItem) FilterValue() string { return l.label }

type worktreeItem struct {
	path       string
	branch     string
	head       string
	identifier string
	slotIdx    int
	slotStatus AgentStatus
}

func (w worktreeItem) Title() string {
	slot := ""
	if w.slotIdx >= 0 {
		var style lipgloss.Style
		switch w.slotStatus {
		case AgentRunning:
			style = slotRunningStyle
		case AgentWaiting:
			style = slotWaitingStyle
		case AgentIdle:
			style = slotIdleStyle
		default:
			style = slotEmptyStyle
		}
		slot = style.Render(fmt.Sprintf(" [%d:%s]", w.slotIdx+1, w.slotStatus.String()))
	}
	ident := ""
	if w.identifier != "" {
		ident = issueIdentStyle.Render(w.identifier) + " "
	}
	return fmt.Sprintf("%s%s%s", ident, w.branch, slot)
}

func (w worktreeItem) Description() string {
	short := w.head
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("%s  %s", short, w.path)
}

func (w worktreeItem) FilterValue() string {
	return w.branch + " " + w.identifier + " " + w.path
}

type worktreeSeparator struct {
	label string
}

func (w worktreeSeparator) Title() string       { return commentDimStyle.Render("── " + w.label + " ──") }
func (w worktreeSeparator) Description() string { return "" }
func (w worktreeSeparator) FilterValue() string { return "" }

func statusIcon(stateType string) string {
	switch stateType {
	case "backlog":
		return lipgloss.NewStyle().Foreground(subtleColor).Render("○")
	case "unstarted":
		return lipgloss.NewStyle().Foreground(dimColor).Render("○")
	case "started":
		return lipgloss.NewStyle().Foreground(yellowColor).Render("●")
	case "completed":
		return lipgloss.NewStyle().Foreground(greenColor).Render("✓")
	case "cancelled":
		return lipgloss.NewStyle().Foreground(mutedColor).Render("✗")
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
