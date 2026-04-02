# linear-worktree

Terminal UI for browsing Linear issues and launching git worktrees with Claude Code sessions.

## Commands

```bash
go build -o linear-worktree .   # Build
go test ./...                    # Run all tests
./linear-worktree               # Run (requires Linear API key)
./linear-worktree --demo        # Run with mock data, no API key needed
LWT_DEBUG=1 ./linear-worktree   # Debug logging to stderr
```

## Architecture

Bubble Tea TUI — single `main` package, no subdirectories.

| File | Purpose |
|------|---------|
| `main.go` | Entry point, flag parsing |
| `model.go` | Model constructor, `Init()` |
| `model_state.go` | Model struct, styles, types, keybindings |
| `model_update.go` | `Update()` — all key/msg handling |
| `model_view.go` | `View()` — all rendering |
| `model_commands.go` | Tea commands (fetch, async ops) |
| `model_list_actions.go` | Issue list operations (assign, comment, filter) |
| `model_detail.go` | Detail panel rendering |
| `model_pickers.go` | Filter/project picker UI |
| `model_settings.go` | Settings form (huh-based) |
| `linear.go` | Linear GraphQL API client |
| `config.go` | Config loading/saving, keychain migration |
| `keyring.go` | OS keychain wrapper for API key storage |
| `worktree.go` | Git worktree operations |
| `cmux.go` | Cmux socket API client, E-layout pane management |
| `launcher.go` | Claude Code / tmux session launching |
| `demo.go` | Demo mode mock data |

## Code Style

- **UI framework**: charmbracelet stack only (bubbles, bubbletea, lipgloss, huh, glamour). No custom widgets.
- All code in `package main` — no internal packages.
- Tests use `_test.go` suffix in same package.

## Key Patterns

- API key stored in OS keychain via `go-keyring`, not in config file. Env var `LINEAR_API_KEY` as fallback.
- Config at `~/.config/linear-worktree/config.json`.
- Cmux integration for E-layout (TUI left, Claude sessions right). Falls back to tmux without cmux.
- `FilterMode` enum controls issue list filtering (assigned/all/todo/in-progress/unassigned).

## Testing

```bash
go test ./...                            # All tests
go test -run TestFoo                     # Single test
go test -v ./...                         # Verbose
go test ./... -args -update-goldens      # Refresh golden snapshots
go test ./... -cover                     # Show coverage %
```

### Test categories

| Category | Files | What's tested |
|----------|-------|---------------|
| **API client** | `linear_test.go` | All GraphQL methods (httptest mocks), pagination, auth, sorting |
| **Golden snapshots** | `model_view_test.go`, `testdata/golden/` | View() output for settings, list, empty state. Run with `-update-goldens` to refresh. |
| **Sequence tests** | `model_view_test.go` | Multi-step UI flows: list→detail→back, help toggle, search mode, filter cycling |
| **Detail view** | `model_test.go` | Metadata rendering (SLA fields, comments, sort order), back-navigation |
| **Settings** | `model_test.go` | Form tabs, validation, credential requirements, team resolution |
| **Worktree ops** | `worktree_test.go` | Create/remove/list worktrees, path traversal protection, file copying |
| **Config** | `config_test.go` | Load/save, keyring migration, API key storage |
| **Utilities** | `model_test.go` | Time formatting, URL truncation, prompt building |

### Writing new tests

- **Golden tests**: Add a `TestGoldenView*` function in `model_view_test.go`. Set up model state, call `viewContent(m)`, compare with `normalizeView()` against a golden file. Run `-update-goldens` to create the initial snapshot.
- **Sequence tests**: Add a `TestSequence*` function that sends a series of `tea.KeyMsg` through `m.Update()` and asserts state changes or view content at each step.
- **Detail rendering**: Create an `&Issue{}` with fields set, call `m.buildDetailContent(issue, width)`, assert with `strings.Contains()`.
- **API tests**: Use `httptest.NewServer` returning canned JSON responses, create `LinearClient` pointing at the test server.
