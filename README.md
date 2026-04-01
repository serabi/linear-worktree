# linear-worktree

A TUI for browsing Linear issues and spawning git worktrees with Claude Code sessions in an E-layout.

Built with Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Bubbles](https://github.com/charmbracelet/bubbles) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Install

```bash
cd linear-worktree-go
go mod tidy
go build -o linear-worktree .
```

## Usage

```bash
./linear-worktree
```

On first run, you'll be prompted for your Linear API key and team key.

### The E-Layout (cmux mode)

When running inside cmux, the TUI manages an E-shaped layout:

```
┌──────────┬──────────────────────┐
│          │  worktree 1 (claude) │
│          ├──────────────────────┤
│  TUI     │  worktree 2 (claude) │
│  (this)  ├──────────────────────┤
│          │  worktree 3 (claude) │
└──────────┴──────────────────────┘
```

- Left 1/3: the TUI (always visible)
- Right 2/3: up to 3 stacked Claude Code sessions
- Each slot shows live status (running/idle/waiting)
- Closing a slot auto-expands the remaining ones

Without cmux, falls back to tmux sessions.

## Keybindings

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate issues |
| `/` | Filter/search issues (fuzzy) |
| `Tab` | Cycle filter (assigned → all → todo → in progress) |
| `c` | Create worktree + launch Claude in E-layout slot |
| `w` | Create worktree only |
| `x` | Close worktree slot (cmux mode) |
| `m` | Add comment to selected issue |
| `d` | Toggle detail panel (description + comments) |
| `g` | Open issue in browser |
| `r` | Refresh issues from Linear |
| `s` | Setup / reconfigure |
| `q` | Quit |

## Features

- **Issue browsing** — status icons, priority indicators, identifiers, fuzzy search
- **🌳 Worktree markers** — see which issues already have worktrees
- **E-layout** — TUI on left, up to 3 Claude sessions stacked on right (cmux)
- **Slot status** — live monitoring of each Claude session (running ● / idle ○ / waiting ◐)
- **Hard cap at 3** — prevents overload, close a slot to open a new one
- **Comments** — post comments to Linear issues directly from the TUI (`m` key)
- **Detail panel** — toggle with `d` to see full issue info + recent comments
- **Worktree management** — auto-creates branch, copies config files (.env, .claude/)
- **Fallback** — uses tmux if cmux isn't available

## Config

Stored at `~/.config/linear-worktree/config.json`:

```json
{
  "linear_api_key": "lin_api_...",
  "team_id": "...",
  "team_key": "TSCODE",
  "worktree_base_dir": "../worktrees",
  "copy_files": [".env", ".envrc"],
  "copy_dirs": [".claude"],
  "claude_command": "claude",
  "branch_prefix": "feature/"
}
```

## Architecture

```
main.go          — entry point
config.go        — config loading/saving
linear.go        — Linear GraphQL API client (issues, comments, status updates)
worktree.go      — git worktree CRUD + file sync
launcher.go      — Claude Code / tmux fallback launcher
cmux.go          — cmux Unix socket client + E-layout PaneManager
model.go         — Bubble Tea TUI (model, update, view)
*_test.go        — tests for all modules
```

## Tests

```bash
go test ./... -v
```
