package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{FilterInProgress, FilterAssigned}, // wraps around
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

	issues, err := client.GetIssues("team-1", FilterAll)
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

	if _, err := client.GetIssues("team-abc-123", FilterAll); err != nil {
		t.Fatalf("GetIssues() error: %v", err)
	}

	if receivedVars == nil {
		t.Fatal("expected variables to be sent, got nil")
	}
	if receivedVars["teamID"] != "team-abc-123" {
		t.Errorf("expected teamID variable 'team-abc-123', got %v", receivedVars["teamID"])
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
