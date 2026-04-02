package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilterModeNext(t *testing.T) {
	tests := []struct {
		mode     FilterMode
		expected FilterMode
	}{
		{FilterAssigned, FilterAll},
		{FilterAll, FilterTodo},
		{FilterTodo, FilterInProgress},
		{FilterInProgress, FilterUnassigned},
		{FilterUnassigned, FilterAssigned}, // wraps around
	}

	for _, tt := range tests {
		result := tt.mode.Next()
		if result != tt.expected {
			t.Errorf("FilterMode(%d).Next() = %d, want %d", tt.mode, result, tt.expected)
		}
	}
}

func TestFilterModeString(t *testing.T) {
	if s := FilterAssigned.String(); s == "" {
		t.Error("FilterAssigned.String() should not be empty")
	}
	if s := FilterAll.String(); s == "" {
		t.Error("FilterAll.String() should not be empty")
	}
	if s := FilterUnassigned.String(); s != "∅ Unassigned" {
		t.Errorf("FilterUnassigned.String() = %q, want %q", s, "∅ Unassigned")
	}
}

// mockLinearServer creates a test server that responds to Linear GraphQL queries.
func mockLinearServer(handler func(query string, vars map[string]any) (int, interface{})) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		status, data := handler(body.Query, body.Variables)
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": data}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
}

func TestLinearClientGetTeams(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"teams": map[string]interface{}{
				"nodes": []map[string]string{
					{"id": "team-1", "name": "Test Team", "key": "TEST"},
					{"id": "team-2", "name": "Other Team", "key": "OTHER"},
				},
			},
		}
	})
	defer server.Close()

	client := &LinearClient{
		apiKey: "test-key",
		client: server.Client(),
	}
	// Override the URL by using the test server
	origURL := linearAPIURL
	// We need to patch the client to use our server URL
	// Since linearAPIURL is a const, we'll test via the transport
	client.client = &http.Client{
		Transport: &rewriteTransport{
			base:    server.Client().Transport,
			destURL: server.URL,
		},
	}
	_ = origURL

	teams, err := client.GetTeams()
	if err != nil {
		t.Fatalf("GetTeams() error: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams[0].Key != "TEST" {
		t.Errorf("expected team key TEST, got %s", teams[0].Key)
	}
}

func TestLinearClientGetIssues(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"id":         "issue-1",
						"identifier": "TEST-123",
						"title":      "Fix the bug",
						"priority":   2,
						"state":      map[string]string{"name": "In Progress", "type": "started", "color": "#f00"},
					},
					{
						"id":         "issue-2",
						"identifier": "TEST-456",
						"title":      "Add feature",
						"priority":   3,
						"state":      map[string]string{"name": "Todo", "type": "unstarted", "color": "#ccc"},
					},
				},
			},
		}
	})
	defer server.Close()

	client := &LinearClient{
		apiKey: "test-key",
		client: &http.Client{
			Transport: &rewriteTransport{
				base:    server.Client().Transport,
				destURL: server.URL,
			},
		},
	}

	issues, _, err := client.GetIssues("team-1", FilterAll, SortUpdatedAt, "")
	if err != nil {
		t.Fatalf("GetIssues() error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].Identifier != "TEST-123" {
		t.Errorf("expected TEST-123, got %s", issues[0].Identifier)
	}
	if issues[1].Priority != 3 {
		t.Errorf("expected priority 3, got %d", issues[1].Priority)
	}
}

func TestLinearClientAddComment(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"commentCreate": map[string]interface{}{
				"success": true,
			},
		}
	})
	defer server.Close()

	client := &LinearClient{
		apiKey: "test-key",
		client: &http.Client{
			Transport: &rewriteTransport{
				base:    server.Client().Transport,
				destURL: server.URL,
			},
		},
	}

	err := client.AddComment("issue-1", "This is a test comment")
	if err != nil {
		t.Fatalf("AddComment() error: %v", err)
	}
}

func TestLinearClientGetComments(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issue": map[string]interface{}{
				"comments": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{
							"id":        "comment-1",
							"body":      "First comment",
							"createdAt": "2026-03-31T12:00:00Z",
							"user":      map[string]string{"displayName": "Sarah", "name": "sarah"},
						},
					},
				},
			},
		}
	})
	defer server.Close()

	client := &LinearClient{
		apiKey: "test-key",
		client: &http.Client{
			Transport: &rewriteTransport{
				base:    server.Client().Transport,
				destURL: server.URL,
			},
		},
	}

	comments, err := client.GetComments("issue-1")
	if err != nil {
		t.Fatalf("GetComments() error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "First comment" {
		t.Errorf("expected 'First comment', got '%s'", comments[0].Body)
	}
}

func TestGraphQLVariablesAreSent(t *testing.T) {
	var receivedVars map[string]any
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		receivedVars = vars
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{},
			},
		}
	})
	defer server.Close()

	client := &LinearClient{
		apiKey: "test-key",
		client: &http.Client{
			Transport: &rewriteTransport{
				base:    server.Client().Transport,
				destURL: server.URL,
			},
		},
	}

	if _, _, err := client.GetIssues("team-abc-123", FilterAll, SortUpdatedAt, ""); err != nil {
		t.Fatalf("GetIssues() error: %v", err)
	}

	if receivedVars == nil {
		t.Fatal("expected variables to be sent, got nil")
	}
	if receivedVars["teamID"] != "team-abc-123" {
		t.Errorf("expected teamID variable 'team-abc-123', got %v", receivedVars["teamID"])
	}
}

func testClient(server *httptest.Server) *LinearClient {
	return &LinearClient{
		apiKey: "test-key",
		client: &http.Client{
			Transport: &rewriteTransport{
				base:    server.Client().Transport,
				destURL: server.URL,
			},
		},
	}
}

func TestLinearClientGetViewer(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"viewer": map[string]string{
				"id": "user-1", "name": "Sarah", "displayName": "Sarah Wolff", "email": "sarah@test.com",
			},
		}
	})
	defer server.Close()

	viewer, err := testClient(server).GetViewer()
	if err != nil {
		t.Fatalf("GetViewer() error: %v", err)
	}
	if viewer.Name != "Sarah" {
		t.Errorf("expected name Sarah, got %s", viewer.Name)
	}
	if viewer.Email != "sarah@test.com" {
		t.Errorf("expected email sarah@test.com, got %s", viewer.Email)
	}
}

func TestLinearClientGetProjects(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"projects": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "proj-1", "name": "Auth Rewrite", "state": "started", "progress": 0.5},
					{"id": "proj-2", "name": "API v2", "state": "planned", "progress": 0.0},
				},
			},
		}
	})
	defer server.Close()

	projects, err := testClient(server).GetProjects("team-1")
	if err != nil {
		t.Fatalf("GetProjects() error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "Auth Rewrite" {
		t.Errorf("expected Auth Rewrite, got %s", projects[0].Name)
	}
	if projects[0].Progress != 0.5 {
		t.Errorf("expected progress 0.5, got %f", projects[0].Progress)
	}
}

func TestLinearClientGetIssuesByProject(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "issue-1", "identifier": "TEST-1", "title": "Auth bug"},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"endCursor":   "",
				},
			},
		}
	})
	defer server.Close()

	issues, pageInfo, err := testClient(server).GetIssuesByProject("team-1", "proj-1", "", FilterAll, SortUpdatedAt)
	if err != nil {
		t.Fatalf("GetIssuesByProject() error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if pageInfo.HasNextPage {
		t.Error("expected no next page")
	}
}

func TestLinearClientGetIssuesWithNoProject(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "issue-1", "identifier": "TEST-1", "title": "No project issue"},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"endCursor":   "",
				},
			},
		}
	})
	defer server.Close()

	issues, pageInfo, err := testClient(server).GetIssuesWithNoProject("team-1", FilterAll, SortUpdatedAt, "")
	if err != nil {
		t.Fatalf("GetIssuesWithNoProject() error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if pageInfo.HasNextPage {
		t.Error("expected no next page")
	}
}

func TestLinearClientGetLabels(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issueLabels": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "label-1", "name": "Bug", "color": "#ff0000"},
					{"id": "label-2", "name": "Feature", "color": "#00ff00"},
					{"id": "label-3", "name": "Improvement", "color": "#0000ff"},
				},
			},
		}
	})
	defer server.Close()

	labels, err := testClient(server).GetLabels("team-1")
	if err != nil {
		t.Fatalf("GetLabels() error: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}
	if labels[0].Name != "Bug" {
		t.Errorf("expected Bug, got %s", labels[0].Name)
	}
	if labels[0].Color != "#ff0000" {
		t.Errorf("expected #ff0000, got %s", labels[0].Color)
	}
}

func TestLinearClientGetIssuesByLabel(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "issue-1", "identifier": "TEST-1", "title": "Bug fix"},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"endCursor":   "",
				},
			},
		}
	})
	defer server.Close()

	issues, pageInfo, err := testClient(server).GetIssuesByLabel("team-1", "label-1", "", FilterAll, SortUpdatedAt)
	if err != nil {
		t.Fatalf("GetIssuesByLabel() error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if pageInfo.HasNextPage {
		t.Error("expected no next page")
	}
}

func TestLinearClientSearchIssues(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		if vars["term"] != "auth bug" {
			t.Errorf("expected term 'auth bug', got %v", vars["term"])
		}
		return 200, map[string]interface{}{
			"searchIssues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "issue-1", "identifier": "TEST-1", "title": "Auth bug fix"},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"endCursor":   "",
				},
			},
		}
	})
	defer server.Close()

	issues, _, err := testClient(server).SearchIssues("auth bug", "team-1", 50, "")
	if err != nil {
		t.Fatalf("SearchIssues() error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

func TestLinearClientSearchIssueByBranch(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issueVcsBranchSearch": map[string]interface{}{
				"id": "issue-1", "identifier": "TEST-1", "title": "Branch issue",
			},
		}
	})
	defer server.Close()

	issue, err := testClient(server).SearchIssueByBranch("feature/test-1")
	if err != nil {
		t.Fatalf("SearchIssueByBranch() error: %v", err)
	}
	if issue == nil {
		t.Fatal("expected issue, got nil")
	}
	if issue.Identifier != "TEST-1" {
		t.Errorf("expected TEST-1, got %s", issue.Identifier)
	}
}

func TestLinearClientUpdateIssueAssignee(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issueUpdate": map[string]interface{}{
				"success": true,
			},
		}
	})
	defer server.Close()

	err := testClient(server).UpdateIssueAssignee("issue-1", "user-1")
	if err != nil {
		t.Fatalf("UpdateIssueAssignee() error: %v", err)
	}
}

func TestLinearClientPagination(t *testing.T) {
	callCount := 0
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		callCount++
		hasNext := callCount == 1
		cursor := ""
		if hasNext {
			cursor = "cursor-1"
		}
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": fmt.Sprintf("issue-%d", callCount), "identifier": fmt.Sprintf("TEST-%d", callCount)},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": hasNext,
					"endCursor":   cursor,
				},
			},
		}
	})
	defer server.Close()

	client := testClient(server)

	// First page
	issues, pageInfo, err := client.GetIssues("team-1", FilterAll, SortUpdatedAt, "")
	if err != nil {
		t.Fatalf("page 1 error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue on page 1, got %d", len(issues))
	}
	if !pageInfo.HasNextPage {
		t.Error("expected hasNextPage=true on page 1")
	}
	if pageInfo.EndCursor != "cursor-1" {
		t.Errorf("expected cursor-1, got %s", pageInfo.EndCursor)
	}

	// Second page
	issues2, pageInfo2, err := client.GetIssues("team-1", FilterAll, SortUpdatedAt, pageInfo.EndCursor)
	if err != nil {
		t.Fatalf("page 2 error: %v", err)
	}
	if len(issues2) != 1 {
		t.Fatalf("expected 1 issue on page 2, got %d", len(issues2))
	}
	if pageInfo2.HasNextPage {
		t.Error("expected hasNextPage=false on page 2")
	}
}

func TestLinearClientGetIssueByID(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		id, _ := vars["id"].(string)
		if id != "issue-42" {
			t.Errorf("expected id 'issue-42', got %q", id)
		}
		return 200, map[string]interface{}{
			"issue": map[string]interface{}{
				"id": "issue-42", "identifier": "TEST-42", "title": "Fetched issue",
			},
		}
	})
	defer server.Close()

	issue, err := testClient(server).GetIssueByID("issue-42")
	if err != nil {
		t.Fatalf("GetIssueByID() error: %v", err)
	}
	if issue == nil {
		t.Fatal("expected issue, got nil")
	}
	if issue.Identifier != "TEST-42" {
		t.Errorf("expected TEST-42, got %s", issue.Identifier)
	}
}

func TestLinearClientGetIssueByIDNotFound(t *testing.T) {
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		return 200, map[string]interface{}{
			"issue": nil,
		}
	})
	defer server.Close()

	_, err := testClient(server).GetIssueByID("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing issue, got nil")
	}
}

func TestSortModeNext(t *testing.T) {
	tests := []struct {
		mode     SortMode
		expected SortMode
	}{
		{SortUpdatedAt, SortCreatedAt},
		{SortCreatedAt, SortPriority},
		{SortPriority, SortUpdatedAt},
	}
	for _, tt := range tests {
		result := tt.mode.Next()
		if result != tt.expected {
			t.Errorf("SortMode(%d).Next() = %d, want %d", tt.mode, result, tt.expected)
		}
	}
}

func TestSortModeString(t *testing.T) {
	tests := []struct {
		mode SortMode
		want string
	}{
		{SortUpdatedAt, "Updated"},
		{SortCreatedAt, "Created"},
		{SortPriority, "Priority"},
		{SortMode(99), "?"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("SortMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestGetIssuesWithPrioritySort(t *testing.T) {
	var capturedVars map[string]any
	var capturedQuery string
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		capturedVars = vars
		capturedQuery = query
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes":    []interface{}{},
				"pageInfo": map[string]interface{}{"hasNextPage": false, "endCursor": ""},
			},
		}
	})
	defer server.Close()

	_, _, err := testClient(server).GetIssues("team-1", FilterAll, SortPriority, "")
	if err != nil {
		t.Fatalf("GetIssues() error: %v", err)
	}

	if capturedVars["sort"] == nil {
		t.Fatal("expected 'sort' variable for priority sort mode")
	}
	if !strings.Contains(capturedQuery, "$sort") {
		t.Error("expected query to contain $sort variable for priority sort")
	}
}

func TestGetIssuesWithCreatedAtSort(t *testing.T) {
	var capturedQuery string
	server := mockLinearServer(func(query string, vars map[string]any) (int, interface{}) {
		capturedQuery = query
		return 200, map[string]interface{}{
			"issues": map[string]interface{}{
				"nodes":    []interface{}{},
				"pageInfo": map[string]interface{}{"hasNextPage": false, "endCursor": ""},
			},
		}
	})
	defer server.Close()

	_, _, err := testClient(server).GetIssues("team-1", FilterAll, SortCreatedAt, "")
	if err != nil {
		t.Fatalf("GetIssues() error: %v", err)
	}

	if !strings.Contains(capturedQuery, "createdAt") {
		t.Error("expected query to contain 'createdAt' orderBy for created sort")
	}
}

func TestAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]string{"id": "1", "name": "test"},
			},
		})
	}))
	defer server.Close()

	client := testClient(server)
	client.apiKey = "lin_api_test123"

	_, _ = client.GetViewer()

	if authHeader != "lin_api_test123" {
		t.Errorf("expected 'lin_api_test123', got '%s'", authHeader)
	}
}

// rewriteTransport rewrites requests to go to the test server.
type rewriteTransport struct {
	base    http.RoundTripper
	destURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.destURL[7:] // strip "http://"
	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
