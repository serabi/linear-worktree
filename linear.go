package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const linearAPIURL = "https://api.linear.app/graphql"

type LinearClient struct {
	apiKey string
	client *http.Client
}

type Issue struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Description string `json:"description"`
	Priority   int    `json:"priority"`
	URL        string `json:"url"`
	BranchName string `json:"branchName"`
	State      struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Color string `json:"color"`
	} `json:"state"`
	Assignee *struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Labels struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
}

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

func NewLinearClient(apiKey string) *LinearClient {
	return &LinearClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (lc *LinearClient) query(q string, result interface{}) error {
	return lc.queryWithVars(q, nil, result)
}

func (lc *LinearClient) queryWithVars(q string, vars map[string]any, result interface{}) error {
	payload, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}

	req, err := http.NewRequest("POST", linearAPIURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", lc.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := lc.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("linear API returned %d", resp.StatusCode)
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return err
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("GraphQL error: %s", gqlResp.Errors[0].Message)
	}

	return json.Unmarshal(gqlResp.Data, result)
}

func (lc *LinearClient) GetTeams() ([]Team, error) {
	var result struct {
		Teams struct {
			Nodes []Team `json:"nodes"`
		} `json:"teams"`
	}

	err := lc.query(`
		query {
			teams {
				nodes { id name key }
			}
		}
	`, &result)

	return result.Teams.Nodes, err
}

var issueQueryByFilter = map[FilterMode]string{
	FilterAssigned: `
		query($teamID: String!) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ assignee: { isMe: { eq: true } } },
					{ state: { type: { nin: ["completed", "cancelled"] } } }
				] }
				first: 50
				orderBy: updatedAt
			) {
				nodes {
					id identifier title description priority url branchName
					state { name type color }
					assignee { name displayName }
					labels { nodes { name color } }
				}
			}
		}`,
	FilterAll: `
		query($teamID: String!) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ state: { type: { nin: ["completed", "cancelled"] } } }
				] }
				first: 50
				orderBy: updatedAt
			) {
				nodes {
					id identifier title description priority url branchName
					state { name type color }
					assignee { name displayName }
					labels { nodes { name color } }
				}
			}
		}`,
	FilterTodo: `
		query($teamID: String!) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ state: { type: { eq: "unstarted" } } }
				] }
				first: 50
				orderBy: updatedAt
			) {
				nodes {
					id identifier title description priority url branchName
					state { name type color }
					assignee { name displayName }
					labels { nodes { name color } }
				}
			}
		}`,
	FilterInProgress: `
		query($teamID: String!) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ state: { type: { eq: "started" } } }
				] }
				first: 50
				orderBy: updatedAt
			) {
				nodes {
					id identifier title description priority url branchName
					state { name type color }
					assignee { name displayName }
					labels { nodes { name color } }
				}
			}
		}`,
}

func (lc *LinearClient) GetIssues(teamID string, filter FilterMode) ([]Issue, error) {
	q := issueQueryByFilter[filter]

	var result struct {
		Issues struct {
			Nodes []Issue `json:"nodes"`
		} `json:"issues"`
	}

	err := lc.queryWithVars(q, map[string]any{"teamID": teamID}, &result)
	return result.Issues.Nodes, err
}

func (lc *LinearClient) AddComment(issueID, body string) error {
	q := `
		mutation($issueId: String!, $body: String!) {
			commentCreate(input: {
				issueId: $issueId
				body: $body
			}) {
				success
			}
		}`

	var result struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}

	if err := lc.queryWithVars(q, map[string]any{"issueId": issueID, "body": body}, &result); err != nil {
		return err
	}
	if !result.CommentCreate.Success {
		return fmt.Errorf("comment creation failed")
	}
	return nil
}

func (lc *LinearClient) UpdateIssueState(issueID, stateID string) error {
	q := `
		mutation($id: String!, $stateId: String!) {
			issueUpdate(id: $id, input: { stateId: $stateId }) {
				success
			}
		}`

	var result struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}

	if err := lc.queryWithVars(q, map[string]any{"id": issueID, "stateId": stateID}, &result); err != nil {
		return err
	}
	if !result.IssueUpdate.Success {
		return fmt.Errorf("issue update failed")
	}
	return nil
}

func (lc *LinearClient) GetComments(issueID string) ([]Comment, error) {
	q := `
		query($id: String!) {
			issue(id: $id) {
				comments(first: 20, orderBy: createdAt) {
					nodes {
						id
						body
						createdAt
						user { displayName name }
					}
				}
			}
		}`

	var result struct {
		Issue struct {
			Comments struct {
				Nodes []Comment `json:"nodes"`
			} `json:"comments"`
		} `json:"issue"`
	}

	if err := lc.queryWithVars(q, map[string]any{"id": issueID}, &result); err != nil {
		return nil, err
	}
	return result.Issue.Comments.Nodes, nil
}

func (lc *LinearClient) GetWorkflowStates(teamID string) ([]WorkflowState, error) {
	q := `
		query($teamID: String!) {
			workflowStates(filter: { team: { id: { eq: $teamID } } }) {
				nodes { id name type }
			}
		}`

	var result struct {
		WorkflowStates struct {
			Nodes []WorkflowState `json:"nodes"`
		} `json:"workflowStates"`
	}

	if err := lc.queryWithVars(q, map[string]any{"teamID": teamID}, &result); err != nil {
		return nil, err
	}
	return result.WorkflowStates.Nodes, nil
}

type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	User      struct {
		DisplayName string `json:"displayName"`
		Name        string `json:"name"`
	} `json:"user"`
}

type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type FilterMode int

const (
	FilterAssigned FilterMode = iota
	FilterAll
	FilterTodo
	FilterInProgress
)

func (f FilterMode) String() string {
	switch f {
	case FilterAssigned:
		return "👤 Assigned to me"
	case FilterAll:
		return "📋 All"
	case FilterTodo:
		return "○ Todo"
	case FilterInProgress:
		return "● In Progress"
	default:
		return "?"
	}
}

func (f FilterMode) Next() FilterMode {
	return (f + 1) % 4
}
