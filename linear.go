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
	payload, _ := json.Marshal(map[string]string{"query": q})

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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Linear API returned %d", resp.StatusCode)
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

func (lc *LinearClient) GetIssues(teamID string, filter FilterMode) ([]Issue, error) {
	filters := fmt.Sprintf(`{ team: { id: { eq: "%s" } } }`, teamID)

	switch filter {
	case FilterAssigned:
		filters += `, { assignee: { isMe: { eq: true } } }`
		filters += `, { state: { type: { nin: ["completed", "cancelled"] } } }`
	case FilterTodo:
		filters += `, { state: { type: { eq: "unstarted" } } }`
	case FilterInProgress:
		filters += `, { state: { type: { eq: "started" } } }`
	case FilterAll:
		filters += `, { state: { type: { nin: ["completed", "cancelled"] } } }`
	}

	q := fmt.Sprintf(`
		query {
			issues(
				filter: { and: [%s] }
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
		}
	`, filters)

	var result struct {
		Issues struct {
			Nodes []Issue `json:"nodes"`
		} `json:"issues"`
	}

	err := lc.query(q, &result)
	return result.Issues.Nodes, err
}

func (lc *LinearClient) AddComment(issueID, body string) error {
	q := fmt.Sprintf(`
		mutation {
			commentCreate(input: {
				issueId: "%s"
				body: "%s"
			}) {
				success
			}
		}
	`, issueID, escapeGraphQL(body))

	var result struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}

	if err := lc.query(q, &result); err != nil {
		return err
	}
	if !result.CommentCreate.Success {
		return fmt.Errorf("comment creation failed")
	}
	return nil
}

func (lc *LinearClient) UpdateIssueState(issueID, stateID string) error {
	q := fmt.Sprintf(`
		mutation {
			issueUpdate(id: "%s", input: { stateId: "%s" }) {
				success
			}
		}
	`, issueID, stateID)

	var result struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}

	if err := lc.query(q, &result); err != nil {
		return err
	}
	if !result.IssueUpdate.Success {
		return fmt.Errorf("issue update failed")
	}
	return nil
}

func (lc *LinearClient) GetComments(issueID string) ([]Comment, error) {
	q := fmt.Sprintf(`
		query {
			issue(id: "%s") {
				comments(first: 20, orderBy: createdAt) {
					nodes {
						id
						body
						createdAt
						user { displayName name }
					}
				}
			}
		}
	`, issueID)

	var result struct {
		Issue struct {
			Comments struct {
				Nodes []Comment `json:"nodes"`
			} `json:"comments"`
		} `json:"issue"`
	}

	if err := lc.query(q, &result); err != nil {
		return nil, err
	}
	return result.Issue.Comments.Nodes, nil
}

func (lc *LinearClient) GetWorkflowStates(teamID string) ([]WorkflowState, error) {
	q := fmt.Sprintf(`
		query {
			workflowStates(filter: { team: { id: { eq: "%s" } } }) {
				nodes { id name type }
			}
		}
	`, teamID)

	var result struct {
		WorkflowStates struct {
			Nodes []WorkflowState `json:"nodes"`
		} `json:"workflowStates"`
	}

	if err := lc.query(q, &result); err != nil {
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

func escapeGraphQL(s string) string {
	var result string
	for _, c := range s {
		switch c {
		case '"':
			result += `\"`
		case '\\':
			result += `\\`
		case '\n':
			result += `\n`
		case '\r':
			result += `\r`
		case '\t':
			result += `\t`
		default:
			result += string(c)
		}
	}
	return result
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
