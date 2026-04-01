package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultSocketPath  = "/tmp/cmux.sock"
	defaultMaxSlots    = 3
	absoluteMaxSlots   = 4
)

// CmuxClient manages communication with cmux via its Unix socket API.
type CmuxClient struct {
	socketPath string
	mu         sync.Mutex
	reqID      atomic.Int64
}

type cmuxRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type cmuxResponse struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// WorktreeSlot tracks a worktree pane in the E-layout.
type WorktreeSlot struct {
	Index      int    // 0, 1, or 2
	SurfaceID  string
	Issue      Issue
	WorktreePath string
	Status     AgentStatus
}

type AgentStatus int

const (
	AgentInactive AgentStatus = iota
	AgentRunning
	AgentIdle
	AgentWaiting
)

func (s AgentStatus) String() string {
	switch s {
	case AgentRunning:
		return "●"
	case AgentIdle:
		return "○"
	case AgentWaiting:
		return "◐"
	default:
		return "·"
	}
}

func (s AgentStatus) Label() string {
	switch s {
	case AgentRunning:
		return "running"
	case AgentIdle:
		return "idle"
	case AgentWaiting:
		return "waiting"
	default:
		return "inactive"
	}
}

// LayoutMode determines the pane arrangement.
type LayoutMode int

const (
	LayoutStacked LayoutMode = iota // 2 or 3 slots: vertical stack on the right
	LayoutGrid                      // 4 slots: 2x2 grid on the right
)

// PaneManager tracks the E-layout state.
type PaneManager struct {
	client      *CmuxClient
	workspaceID string
	tuiSurface  string // left pane (the TUI)
	slots       [absoluteMaxSlots]*WorktreeSlot
	maxSlots    int
	layout      LayoutMode
	mu          sync.RWMutex
}

func NewCmuxClient() *CmuxClient {
	path := os.Getenv("CMUX_SOCKET_PATH")
	if path == "" {
		path = defaultSocketPath
	}
	return &CmuxClient{socketPath: path}
}

// Available checks if cmux socket is accessible.
func (c *CmuxClient) Available() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *CmuxClient) send(method string, params map[string]any) (*cmuxResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cmux socket connect: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	id := fmt.Sprintf("lwt-%d", c.reqID.Add(1))
	req := cmuxRequest{ID: id, Method: method, Params: params}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("cmux send: %w", err)
	}

	var resp cmuxResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("cmux recv: %w", err)
	}

	if !resp.OK {
		return &resp, fmt.Errorf("cmux error: %s", resp.Error)
	}

	return &resp, nil
}

func (c *CmuxClient) SplitSurface(workspaceID, surfaceID, direction string) (string, error) {
	resp, err := c.send("surface.split", map[string]any{
		"workspace_id": workspaceID,
		"surface_id":   surfaceID,
		"direction":    direction,
	})
	if err != nil {
		return "", err
	}
	if id, ok := resp.Result["surface_id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("no surface_id in response")
}

func (c *CmuxClient) CloseSurface(workspaceID, surfaceID string) error {
	_, err := c.send("surface.close", map[string]any{
		"workspace_id": workspaceID,
		"surface_id":   surfaceID,
	})
	return err
}

func (c *CmuxClient) SendText(workspaceID, surfaceID, text string) error {
	_, err := c.send("surface.send_text", map[string]any{
		"workspace_id": workspaceID,
		"surface_id":   surfaceID,
		"text":         text,
	})
	return err
}

func (c *CmuxClient) ReadText(workspaceID, surfaceID string, lines int) (string, error) {
	resp, err := c.send("surface.read_text", map[string]any{
		"workspace_id": workspaceID,
		"surface_id":   surfaceID,
		"lines":        lines,
	})
	if err != nil {
		return "", err
	}
	if text, ok := resp.Result["text"].(string); ok {
		return text, nil
	}
	return "", nil
}

// --- PaneManager ---

func NewPaneManager(client *CmuxClient, maxSlots int) *PaneManager {
	if maxSlots < 2 || maxSlots > absoluteMaxSlots {
		maxSlots = defaultMaxSlots
	}
	layout := LayoutStacked
	if maxSlots == 4 {
		layout = LayoutGrid
	}
	return &PaneManager{
		client:      client,
		workspaceID: os.Getenv("CMUX_WORKSPACE_ID"),
		tuiSurface:  os.Getenv("CMUX_SURFACE_ID"),
		maxSlots:    maxSlots,
		layout:      layout,
	}
}

// OpenSlot adds an issue to the next available slot, creates a pane, and launches Claude.
func (pm *PaneManager) OpenSlot(issue Issue, wtPath string, cfg Config) (*WorktreeSlot, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Find first empty slot
	slotIdx := -1
	for i := 0; i < pm.maxSlots; i++ {
		if pm.slots[i] == nil {
			slotIdx = i
			break
		}
	}
	if slotIdx == -1 {
		return nil, fmt.Errorf("all %d worktree slots are full", pm.maxSlots)
	}

	splitTarget, splitDirection := pm.splitStrategy(slotIdx)
	if splitTarget == "" {
		return nil, fmt.Errorf("no surface to split from")
	}

	surfaceID, err := pm.client.SplitSurface(pm.workspaceID, splitTarget, splitDirection)
	if err != nil {
		return nil, fmt.Errorf("split pane: %w", err)
	}

	// Build Claude prompt
	prompt := fmt.Sprintf("You're working on %s: %s", issue.Identifier, issue.Title)
	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		prompt += fmt.Sprintf("\n\nDescription:\n%s", desc)
	}

	// Send cd + claude command to the new pane
	cdCmd := fmt.Sprintf("cd %s\n", wtPath)
	if err := pm.client.SendText(pm.workspaceID, surfaceID, cdCmd); err != nil {
		return nil, fmt.Errorf("send cd: %w", err)
	}

	claudeCmd := fmt.Sprintf("%s --prompt '%s'\n", cfg.ClaudeCommand, escapeShell(prompt))
	if err := pm.client.SendText(pm.workspaceID, surfaceID, claudeCmd); err != nil {
		return nil, fmt.Errorf("send claude: %w", err)
	}

	slot := &WorktreeSlot{
		Index:        slotIdx,
		SurfaceID:    surfaceID,
		Issue:        issue,
		WorktreePath: wtPath,
		Status:       AgentRunning,
	}
	pm.slots[slotIdx] = slot
	return slot, nil
}

// splitStrategy determines which surface to split and in which direction
// based on the layout mode and slot index.
func (pm *PaneManager) splitStrategy(slotIdx int) (target, direction string) {
	if pm.layout == LayoutGrid {
		// 2x2 grid:
		// slot 0: split TUI right → creates top-right
		// slot 1: split slot 0 right → creates top-right-right (but we want top-left, top-right)
		//   Actually: slot 0 = top-right, slot 1 = split TUI's right down (bottom-right)
		//   Then slot 2 = split slot 0 right, slot 3 = split slot 1 right
		// Better approach:
		// slot 0: split TUI right → right pane
		// slot 1: split right pane down → bottom-right (slot 0 becomes top-right)
		// slot 2: split slot 0 (top-right) right → top-far-right
		// slot 3: split slot 1 (bottom-right) right → bottom-far-right
		//
		// Simplest 2x2:
		// slot 0: split TUI right → right half
		// slot 1: split slot 0 down → slot 0 = top-right, slot 1 = bottom-right
		// slot 2: split slot 0 (top-right) right → top-left-right, top-right-right
		// slot 3: split slot 1 (bottom-right) right → bottom-left-right, bottom-right-right
		//
		// Actually cleanest:
		// slot 0: split TUI right
		// slot 1: split slot 0 down → stacked (same as E for 2 slots)
		// slot 2: split slot 0 (now top) right → top row has 2 panes
		// slot 3: split slot 1 (now bottom) right → bottom row has 2 panes
		switch slotIdx {
		case 0:
			return pm.tuiSurface, "right"
		case 1:
			if pm.slots[0] != nil {
				return pm.slots[0].SurfaceID, "down"
			}
		case 2:
			if pm.slots[0] != nil {
				return pm.slots[0].SurfaceID, "right"
			}
		case 3:
			if pm.slots[1] != nil {
				return pm.slots[1].SurfaceID, "right"
			}
		}
		return "", ""
	}

	// Stacked layout (2 or 3 slots): vertical stack on the right
	if slotIdx == 0 {
		return pm.tuiSurface, "right"
	}
	// Split the last occupied slot downward
	for i := slotIdx - 1; i >= 0; i-- {
		if pm.slots[i] != nil {
			return pm.slots[i].SurfaceID, "down"
		}
	}
	return "", ""
}

// CloseSlot closes a worktree pane and frees the slot.
func (pm *PaneManager) CloseSlot(slotIdx int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if slotIdx < 0 || slotIdx >= pm.maxSlots || pm.slots[slotIdx] == nil {
		return fmt.Errorf("slot %d is empty", slotIdx)
	}

	slot := pm.slots[slotIdx]
	err := pm.client.CloseSurface(pm.workspaceID, slot.SurfaceID)
	pm.slots[slotIdx] = nil
	return err
}

// Slots returns a copy of current slot state.
func (pm *PaneManager) Slots() [absoluteMaxSlots]*WorktreeSlot {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.slots
}

// ActiveCount returns how many slots are occupied.
func (pm *PaneManager) ActiveCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	count := 0
	for _, s := range pm.slots {
		if s != nil {
			count++
		}
	}
	return count
}

// PollStatus reads terminal content from each active pane and infers Claude's status.
func (pm *PaneManager) PollStatus() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i, slot := range pm.slots {
		if slot == nil {
			continue
		}
		text, err := pm.client.ReadText(pm.workspaceID, slot.SurfaceID, 5)
		if err != nil {
			continue
		}
		pm.slots[i].Status = inferStatus(text)
	}
}

// inferStatus pattern-matches terminal content to determine Claude's state.
func inferStatus(terminalText string) AgentStatus {
	// Look for Claude Code's UI patterns
	if containsAny(terminalText, "[y/n]", "[Y/n]") {
		return AgentWaiting
	}
	if containsAny(terminalText, "to interrupt", "ctrl+c") {
		return AgentRunning
	}
	if containsAny(terminalText, "❯", ">") {
		return AgentIdle
	}
	return AgentRunning // default to running if we can't tell
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func escapeShell(s string) string {
	// Simple shell escaping — replace single quotes
	result := ""
	for _, c := range s {
		if c == '\'' {
			result += "'\\''";
		} else {
			result += string(c)
		}
	}
	return result
}
