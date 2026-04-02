package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const linearAPIURL = "https://api.linear.app/graphql"

var debugLog = log.New(io.Discard, "[linear] ", log.LstdFlags)

func init() {
	if os.Getenv("LWT_DEBUG") != "" {
		debugLog.SetOutput(os.Stderr)
	}
}

type LinearClient struct {
	apiKey string
	client *http.Client
}

type Issue struct {
	ID          string   `json:"id"`
	Identifier  string   `json:"identifier"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"`
	URL         string   `json:"url"`
	BranchName  string   `json:"branchName"`
	Estimate    *float64 `json:"estimate"`
	DueDate     *string  `json:"dueDate"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	State       struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Color string `json:"color"`
	} `json:"state"`
	Assignee *struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Labels struct {
		Nodes []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	Cycle *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"cycle"`
	Project *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Parent *struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
	} `json:"parent"`
	Children struct {
		Nodes []struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			State      struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"state"`
		} `json:"nodes"`
	} `json:"children"`
	Relations struct {
		Nodes []struct {
			Type         string `json:"type"`
			RelatedIssue struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
			} `json:"relatedIssue"`
		} `json:"nodes"`
	} `json:"relations"`
}

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

func NewLinearClient(apiKey string) *LinearClient {
	return &LinearClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
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

	debugLog.Printf("request: %s", payload)

	req, err := http.NewRequest("POST", linearAPIURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", lc.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := lc.client.Do(req)
	if err != nil {
		debugLog.Printf("HTTP error: %v", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	debugLog.Printf("response [%d]: %s", resp.StatusCode, body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("linear API returned %d: %s", resp.StatusCode, body)
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &gqlResp); err != nil {
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

func (lc *LinearClient) GetTeamByKey(key string) (*Team, error) {
	var result struct {
		Teams struct {
			Nodes []Team `json:"nodes"`
		} `json:"teams"`
	}

	err := lc.queryWithVars(`
		query($key: String!) {
			teams(filter: { key: { eqIgnoreCase: $key } }) {
				nodes { id name key }
			}
		}
	`, map[string]any{"key": key}, &result)
	if err != nil {
		return nil, err
	}

	if len(result.Teams.Nodes) == 0 {
		return nil, fmt.Errorf("team with key %q not found", key)
	}
	return &result.Teams.Nodes[0], nil
}

// issueListFields is the lightweight field set for list queries.
// Only includes fields needed for list rendering and list-level actions (open, assign, comment).
// Detail-only fields (description, branchName, estimate, labels, cycle, timestamps,
// parent, relations) are fetched on demand via issueFields.
const issueListFields = `
	id identifier title description priority url branchName dueDate
	estimate createdAt updatedAt
	state { name type color }
	assignee { id name displayName }
	labels { nodes { id name color } }
	cycle { id name }
	project { id name }
	parent { id identifier title }
	children { nodes { id identifier title state { name type } } }
	relations { nodes { type relatedIssue { id identifier title } } }
`

// issueFields is the full field set for detail/search queries.
const issueFields = `
	id identifier title description priority url branchName
	estimate dueDate createdAt updatedAt
	state { name type color }
	assignee { id name displayName }
	labels { nodes { id name color } }
	cycle { id name }
	project { id name }
	parent { id identifier title }
	children { nodes { id identifier title state { name type } } }
	relations { nodes { type relatedIssue { id identifier title } } }
`

func issueSortClause(sort SortMode) (queryVars string, orderClause string) {
	switch sort {
	case SortCreatedAt:
		return "$after: String", "after: $after\n\t\t\t\torderBy: createdAt"
	case SortPriority:
		return "$after: String, $sort: [IssueSortInput!]", "after: $after\n\t\t\t\tsort: $sort"
	default:
		return "$after: String", "after: $after\n\t\t\t\torderBy: updatedAt"
	}
}

func issueSortVars(sort SortMode, after string, vars map[string]any) {
	if after != "" {
		vars["after"] = after
	}
	if sort == SortPriority {
		vars["sort"] = []map[string]any{
			{"priority": map[string]any{"order": "Ascending"}},
		}
	}
}

func issueQueryTemplate(sort SortMode) string {
	sortVars, orderClause := issueSortClause(sort)
	return fmt.Sprintf(`
	query($teamID: ID!, %s) {
		issues(
			filter: { and: [
				{ team: { id: { eq: $teamID } } },
				%%s
			] }
			first: 50
			%s
		) {
			nodes { %s }
			pageInfo { hasNextPage endCursor }
		}
	}`, sortVars, orderClause, issueListFields)
}

var issueFilterByMode = map[FilterMode]string{
	FilterAssigned: `
		{ assignee: { isMe: { eq: true } } },
		{ state: { type: { nin: ["completed", "cancelled"] } } }`,
	FilterAll: `
		{ state: { type: { nin: ["completed", "cancelled"] } } }`,
	FilterTodo: `
		{ state: { type: { eq: "unstarted" } } }`,
	FilterInProgress: `
		{ state: { type: { eq: "started" } } }`,
	FilterUnassigned: `
		{ assignee: { null: true } },
		{ state: { type: { nin: ["completed", "cancelled"] } } }`,
}

func (lc *LinearClient) GetIssues(teamID string, filter FilterMode, sort SortMode, after string) ([]Issue, PageInfo, error) {
	q := fmt.Sprintf(issueQueryTemplate(sort), issueFilterByMode[filter])

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
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
		mutation($id: ID!, $stateId: ID!) {
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
		query($teamID: ID!) {
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

func (lc *LinearClient) GetViewer() (*Viewer, error) {
	var result struct {
		Viewer Viewer `json:"viewer"`
	}

	err := lc.query(`
		query {
			viewer { id name displayName email }
		}
	`, &result)
	if err != nil {
		return nil, err
	}
	return &result.Viewer, nil
}

func (lc *LinearClient) GetProjects(teamID string) ([]Project, error) {
	var result struct {
		Projects struct {
			Nodes []Project `json:"nodes"`
		} `json:"projects"`
	}

	err := lc.queryWithVars(`
		query($teamID: ID!) {
			projects(
				filter: {
					accessibleTeams: { id: { eq: $teamID } }
					state: { nin: ["completed", "cancelled"] }
				}
				first: 50
				orderBy: updatedAt
			) {
				nodes { id name state description progress targetDate }
			}
		}
	`, map[string]any{"teamID": teamID}, &result)
	return result.Projects.Nodes, err
}

func (lc *LinearClient) GetIssuesByProject(teamID, projectID, after string, filter FilterMode, sort SortMode) ([]Issue, PageInfo, error) {
	sortVars, orderClause := issueSortClause(sort)
	q := fmt.Sprintf(`
		query($teamID: ID!, $projectID: ID!, %s) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ project: { id: { eq: $projectID } } },
					%s
				] }
				first: 50
				%s
			) {
				nodes { %s }
				pageInfo { hasNextPage endCursor }
			}
		}`, sortVars, issueFilterByMode[filter], orderClause, issueListFields)

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID, "projectID": projectID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
}

func (lc *LinearClient) GetIssuesWithNoProject(teamID string, filter FilterMode, sort SortMode, after string) ([]Issue, PageInfo, error) {
	sortVars, orderClause := issueSortClause(sort)
	q := fmt.Sprintf(`
		query($teamID: ID!, %s) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ project: { null: true } },
					%s
				] }
				first: 50
				%s
			) {
				nodes { `+issueListFields+` }
				pageInfo { hasNextPage endCursor }
			}
		}`, sortVars, issueFilterByMode[filter], orderClause)

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
}

func (lc *LinearClient) GetLabels(teamID string) ([]IssueLabel, error) {
	var all []IssueLabel
	var after string

	for range 5 {
		var result struct {
			IssueLabels struct {
				Nodes    []IssueLabel `json:"nodes"`
				PageInfo PageInfo     `json:"pageInfo"`
			} `json:"issueLabels"`
		}

		vars := map[string]any{"teamID": teamID}
		if after != "" {
			vars["after"] = after
		}

		err := lc.queryWithVars(`
			query($teamID: ID!, $after: String) {
				issueLabels(
					filter: { team: { id: { eq: $teamID } } }
					first: 50
					after: $after
				) {
					nodes { id name color }
					pageInfo { hasNextPage endCursor }
				}
			}
		`, vars, &result)
		if err != nil {
			return all, err
		}

		all = append(all, result.IssueLabels.Nodes...)
		if !result.IssueLabels.PageInfo.HasNextPage {
			break
		}
		after = result.IssueLabels.PageInfo.EndCursor
	}
	return all, nil
}

func (lc *LinearClient) GetIssuesByLabel(teamID, labelID, after string, filter FilterMode, sort SortMode) ([]Issue, PageInfo, error) {
	sortVars, orderClause := issueSortClause(sort)
	q := fmt.Sprintf(`
		query($teamID: ID!, $labelID: ID!, %s) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ labels: { id: { eq: $labelID } } },
					%s
				] }
				first: 50
				%s
			) {
				nodes { %s }
				pageInfo { hasNextPage endCursor }
			}
		}`, sortVars, issueFilterByMode[filter], orderClause, issueListFields)

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID, "labelID": labelID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
}

func (lc *LinearClient) GetIssuesByProjectAndLabel(teamID, projectID, labelID, after string, filter FilterMode, sort SortMode) ([]Issue, PageInfo, error) {
	sortVars, orderClause := issueSortClause(sort)
	q := fmt.Sprintf(`
		query($teamID: ID!, $projectID: ID!, $labelID: ID!, %s) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ project: { id: { eq: $projectID } } },
					{ labels: { id: { eq: $labelID } } },
					%s
				] }
				first: 50
				%s
			) {
				nodes { %s }
				pageInfo { hasNextPage endCursor }
			}
		}`, sortVars, issueFilterByMode[filter], orderClause, issueListFields)

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID, "projectID": projectID, "labelID": labelID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
}

func (lc *LinearClient) GetIssuesWithNoProjectAndLabel(teamID, labelID string, filter FilterMode, sort SortMode, after string) ([]Issue, PageInfo, error) {
	sortVars, orderClause := issueSortClause(sort)
	q := fmt.Sprintf(`
		query($teamID: ID!, $labelID: ID!, %s) {
			issues(
				filter: { and: [
					{ team: { id: { eq: $teamID } } },
					{ project: { null: true } },
					{ labels: { id: { eq: $labelID } } },
					%s
				] }
				first: 50
				%s
			) {
				nodes { `+issueListFields+` }
				pageInfo { hasNextPage endCursor }
			}
		}`, sortVars, issueFilterByMode[filter], orderClause)

	var result struct {
		Issues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"issues"`
	}

	vars := map[string]any{"teamID": teamID, "labelID": labelID}
	issueSortVars(sort, after, vars)

	err := lc.queryWithVars(q, vars, &result)
	return result.Issues.Nodes, result.Issues.PageInfo, err
}

func (lc *LinearClient) SearchIssues(term, teamID string, first int, after string) ([]Issue, PageInfo, error) {
	q := fmt.Sprintf(`
		query($term: String!, $teamID: String, $first: Int, $after: String) {
			searchIssues(
				term: $term
				teamId: $teamID
				first: $first
				after: $after
			) {
				nodes { %s }
				pageInfo { hasNextPage endCursor }
				totalCount
			}
		}`, issueFields)

	var result struct {
		SearchIssues struct {
			Nodes    []Issue  `json:"nodes"`
			PageInfo PageInfo `json:"pageInfo"`
		} `json:"searchIssues"`
	}

	vars := map[string]any{"term": term, "first": first}
	if teamID != "" {
		vars["teamID"] = teamID
	}
	if after != "" {
		vars["after"] = after
	}

	err := lc.queryWithVars(q, vars, &result)
	return result.SearchIssues.Nodes, result.SearchIssues.PageInfo, err
}

func (lc *LinearClient) GetIssueByID(id string) (*Issue, error) {
	q := fmt.Sprintf(`
		query($id: String!) {
			issue(id: $id) {
				%s
			}
		}`, issueFields)

	var result struct {
		Issue *Issue `json:"issue"`
	}

	err := lc.queryWithVars(q, map[string]any{"id": id}, &result)
	if err != nil {
		return nil, err
	}
	if result.Issue == nil {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	return result.Issue, nil
}

func (lc *LinearClient) SearchIssueByBranch(branchName string) (*Issue, error) {
	q := fmt.Sprintf(`
		query($branchName: String!) {
			issueVcsBranchSearch(branchName: $branchName) {
				%s
			}
		}`, issueFields)

	var result struct {
		IssueVcsBranchSearch *Issue `json:"issueVcsBranchSearch"`
	}

	err := lc.queryWithVars(q, map[string]any{"branchName": branchName}, &result)
	if err != nil {
		return nil, err
	}
	return result.IssueVcsBranchSearch, nil
}

func (lc *LinearClient) UpdateIssueAssignee(issueID, assigneeID string) error {
	q := `
		mutation($id: ID!, $assigneeId: ID!) {
			issueUpdate(id: $id, input: { assigneeId: $assigneeId }) {
				success
			}
		}`

	var result struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}

	if err := lc.queryWithVars(q, map[string]any{"id": issueID, "assigneeId": assigneeID}, &result); err != nil {
		return err
	}
	if !result.IssueUpdate.Success {
		return fmt.Errorf("assignee update failed")
	}
	return nil
}

func (lc *LinearClient) UnassignIssue(issueID string) error {
	q := `
		mutation($id: ID!) {
			issueUpdate(id: $id, input: { assigneeId: null }) {
				success
			}
		}`

	var result struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}

	if err := lc.queryWithVars(q, map[string]any{"id": issueID}, &result); err != nil {
		return err
	}
	if !result.IssueUpdate.Success {
		return fmt.Errorf("unassign failed")
	}
	return nil
}

type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	User      struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Name        string `json:"name"`
	} `json:"user"`
}

type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type Project struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Description string  `json:"description"`
	Progress    float64 `json:"progress"`
	TargetDate  *string `json:"targetDate"`
}

type IssueLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Viewer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type FilterMode int

const (
	FilterAssigned FilterMode = iota
	FilterAll
	FilterTodo
	FilterInProgress
	FilterUnassigned
)

func (f FilterMode) String() string {
	switch f {
	case FilterAssigned:
		return "Assigned to me"
	case FilterAll:
		return "All"
	case FilterTodo:
		return "Todo"
	case FilterInProgress:
		return "● In Progress"
	case FilterUnassigned:
		return "∅ Unassigned"
	default:
		return "?"
	}
}

func (f FilterMode) Next() FilterMode {
	return (f + 1) % 5
}

type SortMode int

const (
	SortUpdatedAt SortMode = iota
	SortCreatedAt
	SortPriority
)

func (s SortMode) String() string {
	switch s {
	case SortUpdatedAt:
		return "Updated"
	case SortCreatedAt:
		return "Created"
	case SortPriority:
		return "Priority"
	default:
		return "?"
	}
}

func (s SortMode) Next() SortMode {
	return (s + 1) % 3
}
