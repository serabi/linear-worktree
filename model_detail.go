package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"golang.org/x/term"
)

type detailRenderContext struct {
	width   int
	divider string
}

func newDetailRenderContext(width int) detailRenderContext {
	return detailRenderContext{
		width: width,
		divider: lipgloss.NewStyle().
			Foreground(faintColor).
			Width(width).
			Render(strings.Repeat("─", width)),
	}
}

func (ctx detailRenderContext) dim(text string) string {
	return commentDimStyle.Render(text)
}

func (ctx detailRenderContext) blocker(text string) string {
	return lipgloss.NewStyle().Foreground(redColor).Render(text)
}

func (ctx detailRenderContext) link(text string) string {
	return lipgloss.NewStyle().Foreground(identCyanColor).Render(text)
}

func (ctx detailRenderContext) section(title string) string {
	return "\n" + lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED")).
		Render(title) + "\n" + ctx.divider + "\n"
}

func (ctx detailRenderContext) field(label, value string) string {
	return lipgloss.NewStyle().
		Foreground(dimColor).
		Width(14).
		Align(lipgloss.Right).
		Render(label) + "  " + value + "\n"
}

func (m Model) renderMarkdown(text string, width int) string {
	style := "notty"
	if term.IsTerminal(int(os.Stdout.Fd())) {
		style = "light"
		if compat.HasDarkBackground {
			style = "dark"
		}
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(text)
	}
	rendered, err := renderer.Render(text)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(text)
	}
	return strings.TrimRight(rendered, "\n")
}

func (m Model) buildDetailContent(issue *Issue, width int) string {
	ctx := newDetailRenderContext(width)
	var b strings.Builder

	m.writeDetailHeader(&b, issue, ctx)
	m.writeDetailMetadata(&b, issue, ctx)
	m.writeDetailRelations(&b, issue, ctx)
	m.writeDetailHierarchy(&b, issue, ctx)
	m.writeDetailDescription(&b, issue, ctx)
	m.writeDetailComments(&b, issue, ctx)

	return b.String()
}

func (m Model) writeDetailHeader(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	b.WriteString(issueIdentStyle.Render(issue.Identifier))
	b.WriteString("  ")

	var stateColor color.Color = yellowColor
	if issue.State.Color != "" {
		stateColor = lipgloss.Color(issue.State.Color)
	}
	b.WriteString(lipgloss.NewStyle().
		Foreground(stateColor).
		Bold(true).
		Render(issue.State.Name))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Width(ctx.width).Render(issue.Title))
	b.WriteString("\n")
}

func (m Model) writeDetailMetadata(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	b.WriteString(ctx.section("Details"))
	if issue.Project != nil {
		b.WriteString(ctx.field("Project", issue.Project.Name))
	}
	if issue.Cycle != nil {
		b.WriteString(ctx.field("Cycle", issue.Cycle.Name))
	}
	if issue.Assignee != nil {
		name := issue.Assignee.DisplayName
		if name == "" {
			name = issue.Assignee.Name
		}
		b.WriteString(ctx.field("Assignee", name))
	}

	priNames := map[int]string{0: "None", 1: "Urgent", 2: "High", 3: "Medium", 4: "Low"}
	priName := priNames[issue.Priority]
	switch issue.Priority {
	case 1:
		priName = urgentStyle.Render(priName)
	case 2:
		priName = highStyle.Render(priName)
	case 3:
		priName = mediumStyle.Render(priName)
	case 4:
		priName = lowStyle.Render(priName)
	}
	b.WriteString(ctx.field("Priority", priName))

	if issue.Estimate != nil {
		b.WriteString(ctx.field("Estimate", fmt.Sprintf("%.0f pts", *issue.Estimate)))
	}
	if issue.DueDate != nil {
		b.WriteString(ctx.field("Due", formatDueDate(*issue.DueDate, ctx)))
	}
	if issue.SLABreachesAt != nil || issue.SLAStartedAt != nil {
		if issue.SLABreachesAt != nil {
			breachStr := formatSLABreach(*issue.SLABreachesAt, issue.SLAHighRiskAt, issue.SLAMediumRiskAt, ctx)
			b.WriteString(ctx.field("SLA Breach", breachStr))
		}
		if issue.SLAType != nil {
			b.WriteString(ctx.field("SLA Scope", humanizeSLAType(*issue.SLAType)))
		}
		if issue.SLAStartedAt != nil {
			b.WriteString(ctx.field("SLA Started", relativeTime(*issue.SLAStartedAt)))
		}
	}
	if len(issue.Labels.Nodes) > 0 {
		pills := make([]string, len(issue.Labels.Nodes))
		for i, label := range issue.Labels.Nodes {
			color := "#888"
			if label.Color != "" {
				color = label.Color
			}
			pills[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(label.Name)
		}
		b.WriteString(ctx.field("Labels", strings.Join(pills, ctx.dim(" / "))))
	}
	if issue.CreatedAt != "" {
		b.WriteString(ctx.field("Created", relativeTime(issue.CreatedAt)))
	}
	if issue.UpdatedAt != "" {
		b.WriteString(ctx.field("Updated", relativeTime(issue.UpdatedAt)))
	}
	if issue.BranchName != "" {
		b.WriteString(ctx.field("Branch", lipgloss.NewStyle().Foreground(greenColor).Render(issue.BranchName)))
	}
	if issue.URL != "" {
		b.WriteString(ctx.field("URL", ctx.link(issue.URL)))
	}
}

func formatDueDate(dueDate string, ctx detailRenderContext) string {
	if t, err := time.Parse("2006-01-02", dueDate); err == nil {
		days := int(time.Until(t).Hours() / 24)
		switch {
		case days < 0:
			return ctx.blocker(fmt.Sprintf("%s (OVERDUE by %dd)", dueDate, -days))
		case days <= 3:
			return lipgloss.NewStyle().
				Foreground(yellowColor).
				Render(fmt.Sprintf("%s (%dd)", dueDate, days))
		default:
			return fmt.Sprintf("%s (%dd)", dueDate, days)
		}
	}
	return dueDate
}

func (m Model) writeDetailRelations(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	if len(issue.Relations.Nodes) == 0 {
		return
	}

	b.WriteString(ctx.section("Relations"))
	for _, relation := range issue.Relations.Nodes {
		prefix := relation.Type
		style := ctx.dim
		switch relation.Type {
		case "blocks":
			prefix = "Blocking"
		case "is blocked by", "blocked":
			prefix = "Blocked by"
			style = ctx.blocker
		case "related":
			prefix = "Related"
		case "duplicate":
			prefix = "Duplicate"
		}
		b.WriteString(fmt.Sprintf(
			"  %s %s\n",
			style(prefix+":"),
			ctx.link(relation.RelatedIssue.Identifier)+ctx.dim(" "+relation.RelatedIssue.Title),
		))
	}
}

func (m Model) writeDetailHierarchy(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	if issue.Parent != nil {
		b.WriteString("\n")
		b.WriteString(ctx.field("Parent", ctx.link(issue.Parent.Identifier)+ctx.dim(" "+issue.Parent.Title)))
	}

	if len(issue.Children.Nodes) == 0 {
		return
	}

	b.WriteString(ctx.section(fmt.Sprintf("Sub-issues [%d/%d]", countCompleted(issue.Children.Nodes), len(issue.Children.Nodes))))
	for _, child := range issue.Children.Nodes {
		b.WriteString(fmt.Sprintf("  %s %s %s\n", statusIcon(child.State.Type), ctx.link(child.Identifier), ctx.dim(child.Title)))
	}
}

func (m Model) writeDetailDescription(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	if issue.Description == "" {
		return
	}
	b.WriteString(ctx.section("Description"))
	b.WriteString(m.renderMarkdown(issue.Description, ctx.width))
	b.WriteString("\n")
}

func (m Model) writeDetailComments(b *strings.Builder, issue *Issue, ctx detailRenderContext) {
	if m.cachedCommentID != issue.ID || len(m.cachedComments) == 0 {
		return
	}

	b.WriteString(ctx.section(fmt.Sprintf("Comments (%d)", len(m.cachedComments))))
	commentSep := lipgloss.NewStyle().
		Foreground(faintColor).
		Render(strings.Repeat("─", ctx.width-4))

	comments := m.cachedComments
	if !m.commentSortAsc {
		comments = make([]Comment, len(m.cachedComments))
		for j := range m.cachedComments {
			comments[len(m.cachedComments)-1-j] = m.cachedComments[j]
		}
	}

	for i, comment := range comments {
		name := comment.User.DisplayName
		if name == "" {
			name = comment.User.Name
		}
		ts := relativeTime(comment.CreatedAt)
		isMe := m.viewer != nil && comment.User.ID == m.viewer.ID

		nameRendered := ctx.dim(name)
		if isMe {
			nameRendered = lipgloss.NewStyle().
				Foreground(greenColor).
				Bold(true).
				Render(name)
		}
		tsRendered := ctx.dim(ts)
		gap := ctx.width - 4 - lipgloss.Width(name) - lipgloss.Width(ts)
		if gap < 2 {
			gap = 2
		}
		b.WriteString("  " + nameRendered + strings.Repeat(" ", gap) + tsRendered + "\n")

		body := m.renderMarkdown(comment.Body, ctx.width-4)
		if isMe {
			for _, line := range strings.Split(body, "\n") {
				b.WriteString(lipgloss.NewStyle().Foreground(greenColor).Render("  |") + " " + line + "\n")
			}
		} else {
			for _, line := range strings.Split(body, "\n") {
				b.WriteString("  " + line + "\n")
			}
		}

		if i < len(comments)-1 {
			b.WriteString("  " + commentSep + "\n")
		}
	}
}

func countCompleted(children []struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	State      struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
}) int {
	n := 0
	for _, child := range children {
		if child.State.Type == "completed" {
			n++
		}
	}
	return n
}

func relativeTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func relativeTimeUntil(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	d := time.Until(t)
	if d < 0 {
		d = -d
		switch {
		case d < time.Minute:
			return "just now"
		case d < time.Hour:
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		case d < 24*time.Hour:
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		case d < 30*24*time.Hour:
			return fmt.Sprintf("%dd ago", int(d.Hours()/24))
		default:
			return t.Format("Jan 2")
		}
	}
	switch {
	case d < time.Minute:
		return "in < 1m"
	case d < time.Hour:
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("in %dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("in %dd", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func isTimePast(iso string) bool {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return false
	}
	return !t.After(time.Now())
}

func formatSLABreach(breachesAt string, highRiskAt, mediumRiskAt *string, ctx detailRenderContext) string {
	t, err := time.Parse(time.RFC3339, breachesAt)
	if err != nil {
		return breachesAt
	}
	timeStr := relativeTimeUntil(breachesAt)
	switch {
	case !t.After(time.Now()):
		return ctx.blocker("BREACHED " + timeStr)
	case time.Until(t) <= 24*time.Hour:
		return ctx.blocker(timeStr)
	case highRiskAt != nil && isTimePast(*highRiskAt):
		return lipgloss.NewStyle().Foreground(yellowColor).Render(timeStr)
	case mediumRiskAt != nil && isTimePast(*mediumRiskAt):
		return lipgloss.NewStyle().Foreground(orangeColor).Render(timeStr)
	default:
		return timeStr
	}
}

func humanizeSLAType(slaType string) string {
	switch slaType {
	case "all":
		return "Calendar Days"
	case "onlyBusinessDays":
		return "Business Days"
	default:
		if len(slaType) == 0 {
			return slaType
		}
		return strings.ToUpper(slaType[:1]) + slaType[1:]
	}
}
