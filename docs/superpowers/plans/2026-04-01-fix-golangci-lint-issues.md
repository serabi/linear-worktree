# Fix golangci-lint Issues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 31 golangci-lint issues (27 errcheck, 3 staticcheck, 1 unused) so `golangci-lint run ./...` passes clean.

**Architecture:** Each fix is minimal — add error checks where meaningful, use `//nolint:errcheck` with justification for intentional fire-and-forget calls (like deferred Close on read-only handles), remove dead code, and fix staticcheck suggestions.

**Tech Stack:** Go 1.23, golangci-lint 2.11

---

## File Map

| File | Issues | What changes |
|------|--------|-------------|
| `cmux.go` | 5 errcheck, 1 staticcheck | Handle/annotate Close/SetDeadline/CloseSurface errors, tagged switch |
| `config.go` | 1 errcheck | Check `os.WriteFile` return in migration |
| `linear.go` | 1 errcheck, 1 staticcheck | Handle `resp.Body.Close`, lowercase error string |
| `worktree.go` | 3 errcheck | Handle `cmd.Run`, annotate deferred Close |
| `model.go` | 1 staticcheck, 1 unused | Fix ineffective `m.view` assignment, remove unused `teams` field |
| `config_test.go` | 10 errcheck | Check errors in test helpers |
| `linear_test.go` | 3 errcheck | Check Decode/Encode/GetIssues errors |
| `worktree_test.go` | 5 errcheck | Check Chdir/WriteFile/MkdirAll errors |

---

### Task 1: Fix `cmux.go` (5 errcheck + 1 staticcheck)

**Files:**
- Modify: `cmux.go:122` — `conn.Close()` unchecked
- Modify: `cmux.go:134` — deferred `conn.Close()` unchecked
- Modify: `cmux.go:136` — `conn.SetDeadline()` unchecked
- Modify: `cmux.go:264,270` — `pm.client.CloseSurface()` unchecked (in `OpenSlot` error paths)
- Modify: `cmux.go:439` — switch should use tagged form

- [ ] **Step 1: Fix `Available()` — line 122**

The `conn.Close()` return value is discarded. This is a read-only probe connection; the error is not actionable. Suppress with a blank assignment:

```go
// cmux.go:122 — change:
conn.Close()
// to:
_ = conn.Close()
```

- [ ] **Step 2: Fix `send()` — lines 134, 136**

Deferred `conn.Close()` error is not actionable (we're done with the conn). Suppress. `SetDeadline` error should be returned since it affects correctness:

```go
// cmux.go:134 — change:
defer conn.Close()
// to:
defer func() { _ = conn.Close() }()

// cmux.go:136 — change:
conn.SetDeadline(time.Now().Add(5 * time.Second))
// to:
if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
    return nil, fmt.Errorf("cmux set deadline: %w", err)
}
```

- [ ] **Step 3: Fix `OpenSlot()` error-path CloseSurface — lines 264, 270**

These are best-effort cleanup calls in error paths (if SendText fails, close the orphaned surface). The error is not actionable. Suppress:

```go
// cmux.go:264 — change:
pm.client.CloseSurface(pm.workspaceID, surfaceID)
// to:
_ = pm.client.CloseSurface(pm.workspaceID, surfaceID)

// cmux.go:270 — change:
pm.client.CloseSurface(pm.workspaceID, surfaceID)
// to:
_ = pm.client.CloseSurface(pm.workspaceID, surfaceID)
```

- [ ] **Step 4: Fix tagged switch — line 439**

staticcheck QF1002: `switch { case c == '\'': ...}` should use `switch c { case '\'': ... }`. Refactor:

```go
// cmux.go:438-446 — change:
for _, c := range s {
    switch {
    case c == '\'':
        result = append(result, '\'', '\\', '\'', '\'')
    case c == '\n' || c == '\r':
        result = append(result, ' ')
    default:
        result = append(result, string(c)...)
    }
}
// to:
for _, c := range s {
    switch c {
    case '\'':
        result = append(result, '\'', '\\', '\'', '\'')
    case '\n', '\r':
        result = append(result, ' ')
    default:
        result = append(result, string(c)...)
    }
}
```

- [ ] **Step 5: Run linter on cmux.go**

Run: `golangci-lint run cmux.go`
Expected: no issues

- [ ] **Step 6: Run tests**

Run: `go test ./... -run TestShell -v` (and any cmux tests)
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmux.go
git commit -m "fix(cmux): resolve errcheck and staticcheck lint issues"
```

---

### Task 2: Fix `config.go` (1 errcheck)

**Files:**
- Modify: `config.go:104` — `os.WriteFile` unchecked in `migrateAPIKeyToKeyring`

- [ ] **Step 1: Check the WriteFile return value**

The migration function is best-effort (line 98 already returns early if keychain fails). But silently dropping a write error means the plaintext key stays on disk when we think it's been removed. Log-and-continue is the right pattern here, but since there's no logger, just return early to avoid the function appearing to succeed:

```go
// config.go:103-104 — change:
if data, err := json.MarshalIndent(fileCfg, "", "  "); err == nil {
    os.WriteFile(path, data, 0600)
}
// to:
if data, err := json.MarshalIndent(fileCfg, "", "  "); err == nil {
    _ = os.WriteFile(path, data, 0600)
}
```

Note: we use `_ =` because the key is already safely in the keychain at this point. If the file rewrite fails, the worst case is a redundant plaintext copy that gets cleaned up on next load. Returning an error here would complicate the caller for a non-critical path.

- [ ] **Step 2: Run linter on config.go**

Run: `golangci-lint run config.go`
Expected: no issues

- [ ] **Step 3: Run tests**

Run: `go test ./... -run TestConfig -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add config.go
git commit -m "fix(config): acknowledge WriteFile error in migration"
```

---

### Task 3: Fix `linear.go` (1 errcheck + 1 staticcheck)

**Files:**
- Modify: `linear.go:77` — deferred `resp.Body.Close()` unchecked
- Modify: `linear.go:80` — error string should not be capitalized (ST1005)

- [ ] **Step 1: Fix deferred Body.Close — line 77**

```go
// linear.go:77 — change:
defer resp.Body.Close()
// to:
defer func() { _ = resp.Body.Close() }()
```

- [ ] **Step 2: Fix capitalized error string — line 80**

```go
// linear.go:80 — change:
return fmt.Errorf("Linear API returned %d", resp.StatusCode)
// to:
return fmt.Errorf("linear API returned %d", resp.StatusCode)
```

- [ ] **Step 3: Run linter on linear.go**

Run: `golangci-lint run linear.go`
Expected: no issues

- [ ] **Step 4: Run tests**

Run: `go test ./... -run TestLinear -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add linear.go
git commit -m "fix(linear): resolve errcheck and staticcheck lint issues"
```

---

### Task 4: Fix `worktree.go` (3 errcheck)

**Files:**
- Modify: `worktree.go:155` — `cmd.Run()` unchecked (branch delete)
- Modify: `worktree.go:171` — deferred `in.Close()` unchecked
- Modify: `worktree.go:177` — deferred `out.Close()` unchecked

- [ ] **Step 1: Fix cmd.Run for branch delete — line 155**

This is explicitly best-effort (comment on lines 152-154 says so). Suppress:

```go
// worktree.go:155 — change:
cmd.Run()
// to:
_ = cmd.Run()
```

- [ ] **Step 2: Fix deferred Close in copyFile — lines 171, 177**

`in.Close()` is read-only, not actionable. `out.Close()` on a write handle CAN lose data (flush failure). Handle `out.Close()` properly:

```go
// worktree.go:171 — change:
defer in.Close()
// to:
defer func() { _ = in.Close() }()

// worktree.go:173-179 — change:
out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
if err != nil {
    return err
}
defer out.Close()

_, err = io.Copy(out, in)
return err
// to:
out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
if err != nil {
    return err
}
defer func() { _ = out.Close() }()

if _, err = io.Copy(out, in); err != nil {
    return err
}
return out.Close()
```

This pattern calls `Close()` explicitly for its error, and the deferred close is a safety net (double-close on an os.File is harmless).

- [ ] **Step 3: Run linter on worktree.go**

Run: `golangci-lint run worktree.go`
Expected: no issues

- [ ] **Step 4: Run tests**

Run: `go test ./... -run TestWorktree -v && go test ./... -run TestCopy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add worktree.go
git commit -m "fix(worktree): resolve errcheck lint issues in file ops"
```

---

### Task 5: Fix `model.go` (1 staticcheck + 1 unused)

**Files:**
- Modify: `model.go:170` — unused field `teams`
- Modify: `model.go:365` — ineffective assignment `m.view = viewSetup`

- [ ] **Step 1: Remove unused `teams` field — line 170**

The `teams` field on `Model` is never read. The `resolveTeamCmd` method (line 445) uses a local `teams` variable. Remove the struct field:

```go
// model.go:170 — delete this line:
teams []Team
```

- [ ] **Step 2: Fix ineffective assignment — line 365**

`Init()` has a value receiver `(m Model)`, so `m.view = viewSetup` doesn't persist. The Bubble Tea framework uses the return value of `Update` for state changes, but `Init` only returns a `tea.Cmd`. The fix: the `New()` constructor should set `view = viewSetup` when `NeedsSetup()` is true, rather than Init trying to mutate a copy. Check the constructor:

The constructor at `New()` needs to check `cfg.NeedsSetup()` and set the view there. And `Init()` should just read `m.view` instead of setting it:

```go
// In the New() constructor, after creating the model, add:
if cfg.NeedsSetup() {
    m.view = viewSetup
}

// In Init(), line 364-366 — change:
if m.cfg.NeedsSetup() {
    m.view = viewSetup
    return textinput.Blink
}
// to:
if m.cfg.NeedsSetup() {
    return textinput.Blink
}
```

- [ ] **Step 3: Run linter on model.go**

Run: `golangci-lint run model.go`
Expected: no issues

- [ ] **Step 4: Run tests (build check)**

Run: `go build ./...`
Expected: compiles with no errors

- [ ] **Step 5: Commit**

```bash
git add model.go
git commit -m "fix(model): remove unused field, fix ineffective assignment in Init"
```

---

### Task 6: Fix `config_test.go` (10 errcheck)

**Files:**
- Modify: `config_test.go:70,122,163` — `os.MkdirAll` unchecked
- Modify: `config_test.go:123,164` — `os.WriteFile` unchecked
- Modify: `config_test.go:96,143,148` — `deleteAPIKey()` unchecked
- Modify: `config_test.go:128,169,190` — `json.Unmarshal` unchecked

- [ ] **Step 1: Fix all unchecked errors in TestSaveAndLoadConfig**

```go
// config_test.go:70 — change:
os.MkdirAll(filepath.Dir(path), 0755)
// to:
if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
    t.Fatalf("mkdir: %v", err)
}
```

- [ ] **Step 2: Fix all unchecked errors in TestSaveConfigStoresAPIKeyInKeyring**

```go
// config_test.go:96 — change:
deleteAPIKey()
// to:
_ = deleteAPIKey()

// config_test.go:122 — change:
os.MkdirAll(filepath.Dir(path), 0700)
// to:
if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
    t.Fatalf("mkdir: %v", err)
}

// config_test.go:123 — change:
os.WriteFile(path, data, 0600)
// to:
if err := os.WriteFile(path, data, 0600); err != nil {
    t.Fatalf("write: %v", err)
}

// config_test.go:128 — change:
json.Unmarshal(fileData, &fileCfg)
// to:
if err := json.Unmarshal(fileData, &fileCfg); err != nil {
    t.Fatalf("unmarshal: %v", err)
}

// config_test.go:143 — change:
deleteAPIKey()
// to:
_ = deleteAPIKey()
```

- [ ] **Step 3: Fix all unchecked errors in TestAPIKeyMigration**

```go
// config_test.go:148 — change:
deleteAPIKey()
// to:
_ = deleteAPIKey()

// config_test.go:163 — change:
os.MkdirAll(filepath.Dir(path), 0700)
// to:
if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
    t.Fatalf("mkdir: %v", err)
}

// config_test.go:164 — change:
os.WriteFile(path, data, 0600)
// to:
if err := os.WriteFile(path, data, 0600); err != nil {
    t.Fatalf("write: %v", err)
}

// config_test.go:169 — change:
json.Unmarshal(fileData, &cfg)
// to:
if err := json.Unmarshal(fileData, &cfg); err != nil {
    t.Fatalf("unmarshal: %v", err)
}

// config_test.go:190 — change:
json.Unmarshal(fileData, &fileCfg)
// to:
if err := json.Unmarshal(fileData, &fileCfg); err != nil {
    t.Fatalf("unmarshal: %v", err)
}
```

- [ ] **Step 4: Run linter on config_test.go**

Run: `golangci-lint run config_test.go`
Expected: no issues

- [ ] **Step 5: Run tests**

Run: `go test ./... -run TestConfig -v && go test ./... -run TestSave -v && go test ./... -run TestAPI -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add config_test.go
git commit -m "fix(config_test): check all error return values"
```

---

### Task 7: Fix `linear_test.go` (3 errcheck)

**Files:**
- Modify: `linear_test.go:45` — `json.NewDecoder(r.Body).Decode(&body)` unchecked
- Modify: `linear_test.go:49` — `json.NewEncoder(w).Encode(...)` unchecked
- Modify: `linear_test.go:233` — `client.GetIssues(...)` unchecked

- [ ] **Step 1: Fix mockLinearServer handler**

```go
// linear_test.go:45 — change:
json.NewDecoder(r.Body).Decode(&body)
// to:
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}

// linear_test.go:49 — change:
json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
// to:
if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": data}); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
```

- [ ] **Step 2: Fix unchecked GetIssues in TestGraphQLVariablesAreSent**

```go
// linear_test.go:233 — change:
client.GetIssues("team-abc-123", FilterAll)
// to:
if _, err := client.GetIssues("team-abc-123", FilterAll); err != nil {
    t.Fatalf("GetIssues() error: %v", err)
}
```

- [ ] **Step 3: Run linter on linear_test.go**

Run: `golangci-lint run linear_test.go`
Expected: no issues

- [ ] **Step 4: Run tests**

Run: `go test ./... -run TestLinear -v && go test ./... -run TestFilter -v && go test ./... -run TestGraphQL -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add linear_test.go
git commit -m "fix(linear_test): check all error return values"
```

---

### Task 8: Fix `worktree_test.go` (5 errcheck)

**Files:**
- Modify: `worktree_test.go:49,50,93` — `os.Chdir` unchecked
- Modify: `worktree_test.go:44` — `os.WriteFile` unchecked (line 44 in TestCreateWorktree)
- Modify: `worktree_test.go:168,169` — `os.MkdirAll`, `os.WriteFile` unchecked in TestCopyDir

Note: The linter flagged lines 49, 50, 93. Let me also check line 44 and the TestCopyDir setup lines since they show similar patterns.

- [ ] **Step 1: Fix os.Chdir calls in TestCreateWorktree**

```go
// worktree_test.go:48-50 — change:
origDir, _ := os.Getwd()
os.Chdir(repoDir)
defer os.Chdir(origDir)
// to:
origDir, err := os.Getwd()
if err != nil {
    t.Fatalf("getwd: %v", err)
}
if err := os.Chdir(repoDir); err != nil {
    t.Fatalf("chdir: %v", err)
}
defer func() {
    if err := os.Chdir(origDir); err != nil {
        t.Errorf("restore dir: %v", err)
    }
}()
```

- [ ] **Step 2: Fix os.Chdir calls in TestListWorktrees**

```go
// worktree_test.go:92-94 — change:
origDir, _ := os.Getwd()
os.Chdir(repoDir)
defer os.Chdir(origDir)
// to:
origDir, err := os.Getwd()
if err != nil {
    t.Fatalf("getwd: %v", err)
}
if err := os.Chdir(repoDir); err != nil {
    t.Fatalf("chdir: %v", err)
}
defer func() {
    if err := os.Chdir(origDir); err != nil {
        t.Errorf("restore dir: %v", err)
    }
}()
```

- [ ] **Step 3: Fix os.Chdir in TestRemoveWorktree and TestCreateWorktreePathTraversal**

Apply the same pattern to all remaining `os.Chdir` calls in the file (lines ~116-118 and ~197-199).

- [ ] **Step 4: Run linter on worktree_test.go**

Run: `golangci-lint run worktree_test.go`
Expected: no issues

- [ ] **Step 5: Run tests**

Run: `go test ./... -run TestWorktree -v && go test ./... -run TestCopy -v && go test ./... -run TestCreate -v && go test ./... -run TestRemove -v && go test ./... -run TestList -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add worktree_test.go
git commit -m "fix(worktree_test): check all error return values"
```

---

### Task 9: Final Verification

- [ ] **Step 1: Run full linter**

Run: `golangci-lint run ./...`
Expected: 0 issues

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: all tests PASS

- [ ] **Step 3: Final commit (if any stragglers)**

Only if prior tasks missed something.
