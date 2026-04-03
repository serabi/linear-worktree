package main

type issuesLoadedMsg struct {
	issues []Issue
	err    error
}

type worktreesLoadedMsg struct {
	branches map[string]bool
}

type worktreeCreatedMsg struct {
	path       string
	identifier string
	err        error
	hookErr    error
}

type claudeLaunchedMsg struct {
	identifier string
	err        error
}

type cmuxSlotOpenedMsg struct {
	slotIdx    int
	identifier string
	wtPath     string
	err        error
}

type teamsLoadedMsg struct {
	err error
}

type setupCompleteMsg struct {
	cfg Config
}

type commentPostedMsg struct {
	identifier string
	err        error
}

type commentsLoadedMsg struct {
	issueID  string
	comments []Comment
	err      error
}

type launchReadyMsg struct {
	issue   Issue
	wtPath  string
	prompt  string
	hookErr error
}

type statusPollMsg struct{}

type viewerLoadedMsg struct {
	viewer *Viewer
	err    error
}

type projectsLoadedMsg struct {
	projects []Project
	err      error
}

type statesLoadedMsg struct {
	states []WorkflowState
	err    error
}

type issueAssignedMsg struct {
	identifier string
	err        error
}

type issueUnassignedMsg struct {
	identifier string
	err        error
}

type issueStateChangedMsg struct {
	identifier string
	err        error
}

type branchIssueFoundMsg struct {
	issue *Issue
}

type issueNavigatedMsg struct {
	issue *Issue
	err   error
}

type detailContentMsg struct {
	issueID string
	content string
}

type searchResultsMsg struct {
	issues []Issue
	err    error
}

type prefetchTickMsg struct {
	seq int
}

type teamSwitchedMsg struct {
	cfg Config
	err error
}

type worktreeRemovedMsg struct {
	identifier string
	err        error
}

type worktreeListLoadedMsg struct {
	worktrees []Worktree
	err       error
}
