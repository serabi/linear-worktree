package main

import "flag"

var demoMode = flag.Bool("demo", false, "Run with mock data (no Linear API needed)")

func DemoConfig() Config {
	return Config{
		LinearAPIKey:  "demo-key",
		TeamID:        "demo-team-id",
		TeamKey:       "ENG",
		Teams:         []TeamEntry{{ID: "demo-team-id", Key: "ENG"}},
		WorktreeBase:  "/tmp/linear-worktree-demo",
		CopyFiles:     []string{".env"},
		CopyDirs:      []string{".claude"},
		ClaudeCommand: "echo",
		ClaudeArgs:    "",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
}

func DemoViewer() *Viewer {
	return &Viewer{
		ID:          "demo-viewer-1",
		Name:        "Alex Chen",
		DisplayName: "Alex Chen",
		Email:       "alex@example.com",
	}
}

func DemoIssues() []Issue {
	est2 := float64(2)
	est3 := float64(3)
	est5 := float64(5)
	est8 := float64(8)
	due1 := "2026-04-10"
	due2 := "2026-04-15"

	issues := []Issue{
		{
			ID: "demo-1", Identifier: "ENG-142", Title: "Add rate limiting to public API endpoints",
			Description: "We need to implement rate limiting on all public-facing API endpoints to prevent abuse and ensure fair usage.\n\n## Requirements\n- Token bucket algorithm\n- Configurable per-endpoint limits\n- Return proper 429 responses with Retry-After header",
			Priority: 1, URL: "https://linear.app/demo/issue/ENG-142",
			BranchName: "feature/eng-142-rate-limiting", CreatedAt: "2026-03-20T10:00:00Z", UpdatedAt: "2026-03-31T14:00:00Z",
			Estimate: &est5, DueDate: &due1,
		},
		{
			ID: "demo-2", Identifier: "ENG-138", Title: "Fix connection pool exhaustion under load",
			Description: "Database connections are being leaked when requests timeout, eventually exhausting the pool.",
			Priority: 1, URL: "https://linear.app/demo/issue/ENG-138",
			BranchName: "fix/eng-138-connection-pool", CreatedAt: "2026-03-18T09:00:00Z", UpdatedAt: "2026-03-30T16:00:00Z",
			Estimate: &est3,
		},
		{
			ID: "demo-3", Identifier: "ENG-155", Title: "Migrate user settings to new schema",
			Description: "Part of the settings v2 migration. Move user preferences from the legacy JSON blob to structured columns.",
			Priority: 2, URL: "https://linear.app/demo/issue/ENG-155",
			BranchName: "feature/eng-155-settings-migration", CreatedAt: "2026-03-25T11:00:00Z", UpdatedAt: "2026-03-31T09:00:00Z",
			Estimate: &est8,
		},
		{
			ID: "demo-4", Identifier: "ENG-160", Title: "Implement webhook retry with exponential backoff",
			Description: "Webhook deliveries should retry on failure with exponential backoff and jitter.",
			Priority: 2, URL: "https://linear.app/demo/issue/ENG-160",
			BranchName: "feature/eng-160-webhook-retry", CreatedAt: "2026-03-27T14:00:00Z", UpdatedAt: "2026-03-31T10:00:00Z",
			Estimate: &est3, DueDate: &due2,
		},
		{
			ID: "demo-5", Identifier: "ENG-148", Title: "Add OpenTelemetry tracing to auth service",
			Description: "Instrument the auth service with OpenTelemetry spans for better observability.",
			Priority: 3, URL: "https://linear.app/demo/issue/ENG-148",
			BranchName: "feature/eng-148-otel-tracing", CreatedAt: "2026-03-22T08:00:00Z", UpdatedAt: "2026-03-29T15:00:00Z",
			Estimate: &est3,
		},
		{
			ID: "demo-6", Identifier: "ENG-163", Title: "Refactor notification preferences into service layer",
			Description: "Extract notification preference logic from the handler into a dedicated service for reuse.",
			Priority: 3, URL: "https://linear.app/demo/issue/ENG-163",
			BranchName: "refactor/eng-163-notification-service", CreatedAt: "2026-03-28T10:00:00Z", UpdatedAt: "2026-03-30T11:00:00Z",
			Estimate: &est2,
		},
		{
			ID: "demo-7", Identifier: "ENG-170", Title: "Update search indexer to handle soft-deleted records",
			Description: "The search index includes soft-deleted records. Filter them out during indexing and add a cleanup job.",
			Priority: 2, URL: "https://linear.app/demo/issue/ENG-170",
			BranchName: "fix/eng-170-search-soft-delete", CreatedAt: "2026-03-30T09:00:00Z", UpdatedAt: "2026-03-31T08:00:00Z",
		},
		{
			ID: "demo-8", Identifier: "ENG-135", Title: "Set up CI pipeline for integration tests",
			Description: "Create a GitHub Actions workflow that runs integration tests against a test database on every PR.",
			Priority: 4, URL: "https://linear.app/demo/issue/ENG-135",
			BranchName: "chore/eng-135-integration-ci", CreatedAt: "2026-03-15T13:00:00Z", UpdatedAt: "2026-03-28T17:00:00Z",
			Estimate: &est5,
		},
		{
			ID: "demo-9", Identifier: "ENG-171", Title: "Add bulk export endpoint for compliance reports",
			Description: "Compliance team needs an endpoint to export user activity logs in CSV format for audit purposes.",
			Priority: 3, URL: "https://linear.app/demo/issue/ENG-171",
			BranchName: "feature/eng-171-bulk-export", CreatedAt: "2026-03-31T08:00:00Z", UpdatedAt: "2026-03-31T12:00:00Z",
			Estimate: &est5,
		},
		{
			ID: "demo-10", Identifier: "ENG-152", Title: "Upgrade Go to 1.23 and update dependencies",
			Description: "Routine maintenance: bump Go version and run dependency updates.",
			Priority: 4, URL: "https://linear.app/demo/issue/ENG-152",
			BranchName: "chore/eng-152-go-upgrade", CreatedAt: "2026-03-24T10:00:00Z", UpdatedAt: "2026-03-29T09:00:00Z",
			Estimate: &est2,
		},
	}

	// Set states
	states := []struct {
		name, typ, color string
	}{
		{"In Progress", "started", "#F59E0B"},
		{"In Progress", "started", "#F59E0B"},
		{"Todo", "unstarted", "#E2E2E2"},
		{"Todo", "unstarted", "#E2E2E2"},
		{"In Progress", "started", "#F59E0B"},
		{"Backlog", "backlog", "#BDBDBD"},
		{"Todo", "unstarted", "#E2E2E2"},
		{"Done", "completed", "#5E6AD2"},
		{"Backlog", "backlog", "#BDBDBD"},
		{"Cancelled", "cancelled", "#95A2B3"},
	}
	for i := range issues {
		issues[i].State.Name = states[i].name
		issues[i].State.Type = states[i].typ
		issues[i].State.Color = states[i].color
	}

	// Set assignees
	alex := struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}{"user-1", "Alex Chen", "Alex Chen"}
	jordan := alex
	jordan.ID = "user-2"
	jordan.Name = "Jordan Rivera"
	jordan.DisplayName = "Jordan Rivera"
	sam := alex
	sam.ID = "user-3"
	sam.Name = "Sam Patel"
	sam.DisplayName = "Sam Patel"

	issues[0].Assignee = &alex
	issues[1].Assignee = &alex
	issues[2].Assignee = &jordan
	issues[4].Assignee = &sam
	issues[5].Assignee = &jordan
	issues[7].Assignee = &sam
	// issues 3, 6, 8, 9 are unassigned

	// Set labels
	issues[0].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-backend", "backend", "#0EA5E9"}, {"lbl-security", "security", "#EF4444"}}
	issues[1].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-bug", "bug", "#EF4444"}, {"lbl-backend", "backend", "#0EA5E9"}}
	issues[2].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-migration", "migration", "#8B5CF6"}}
	issues[3].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-backend", "backend", "#0EA5E9"}}
	issues[4].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-observability", "observability", "#F59E0B"}}
	issues[7].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-infrastructure", "infrastructure", "#6B7280"}}
	issues[8].Labels.Nodes = []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}{{"lbl-compliance", "compliance", "#10B981"}, {"lbl-backend", "backend", "#0EA5E9"}}

	// Set projects
	platformProject := struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{"proj-1", "Platform Hardening"}
	settingsProject := struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{"proj-2", "Settings V2"}

	issues[0].Project = &platformProject
	issues[1].Project = &platformProject
	issues[2].Project = &settingsProject
	issues[4].Project = &platformProject

	// Set a parent/child relationship
	issues[2].Parent = &struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
	}{"demo-parent-1", "ENG-100", "Settings V2 migration epic"}

	return issues
}

func DemoComments() []Comment {
	comments := []Comment{
		{
			ID:        "comment-1",
			Body:      "I did some initial research on rate limiting algorithms. Token bucket seems like the best fit for our use case since it allows short bursts while maintaining an average rate. Redis would work well as the backing store.",
			CreatedAt: "2026-03-28T14:30:00Z",
		},
		{
			ID:        "comment-2",
			Body:      "Agreed on token bucket. We should also make sure the limits are configurable per API key tier, not just per endpoint. Premium customers need higher limits.",
			CreatedAt: "2026-03-29T09:15:00Z",
		},
		{
			ID:        "comment-3",
			Body:      "I pushed an initial implementation to the branch. The middleware is in place and tests pass locally. Still need to wire up the Redis store for distributed rate limiting - right now it is in-memory only.",
			CreatedAt: "2026-03-31T11:00:00Z",
		},
	}

	comments[0].User.ID = "user-1"
	comments[0].User.DisplayName = "Alex Chen"
	comments[0].User.Name = "Alex Chen"

	comments[1].User.ID = "user-2"
	comments[1].User.DisplayName = "Jordan Rivera"
	comments[1].User.Name = "Jordan Rivera"

	comments[2].User.ID = "user-1"
	comments[2].User.DisplayName = "Alex Chen"
	comments[2].User.Name = "Alex Chen"

	return comments
}
