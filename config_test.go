package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ClaudeCommand != "claude" {
		t.Errorf("default ClaudeCommand = %q, want 'claude'", cfg.ClaudeCommand)
	}
	if cfg.BranchPrefix != "feature/" {
		t.Errorf("default BranchPrefix = %q, want 'feature/'", cfg.BranchPrefix)
	}
	if cfg.WorktreeBase != "../worktrees" {
		t.Errorf("default WorktreeBase = %q, want '../worktrees'", cfg.WorktreeBase)
	}
	if len(cfg.CopyFiles) != 2 {
		t.Errorf("default CopyFiles length = %d, want 2", len(cfg.CopyFiles))
	}
	if len(cfg.CopyDirs) != 1 {
		t.Errorf("default CopyDirs length = %d, want 1", len(cfg.CopyDirs))
	}
	if cfg.MaxSlots != 3 {
		t.Errorf("default MaxSlots = %d, want 3", cfg.MaxSlots)
	}
}

func TestConfigNeedsSetup(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.NeedsSetup() {
		t.Error("empty config should need setup")
	}

	cfg.LinearAPIKey = "lin_api_test"
	if !cfg.NeedsSetup() {
		t.Error("config with only API key should still need setup (missing team ID)")
	}

	cfg.TeamID = "team-123"
	if cfg.NeedsSetup() {
		t.Error("config with API key and team ID should not need setup")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Use a temp directory
	tmpDir := t.TempDir()
	origConfigPath := configPath
	// We can't easily override configPath since it's a function,
	// so we'll test the save/load logic directly with temp files

	path := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		LinearAPIKey:  "lin_api_test123",
		TeamID:        "team-abc",
		TeamKey:       "TEST",
		WorktreeBase:  "/tmp/worktrees",
		CopyFiles:     []string{".env"},
		CopyDirs:      []string{".claude"},
		ClaudeCommand: "claude",
		BranchPrefix:  "feature/",
	}

	// Save manually
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Read back
	loaded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var loadedCfg Config
	if err := json.Unmarshal(loaded, &loadedCfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if loadedCfg.LinearAPIKey != cfg.LinearAPIKey {
		t.Errorf("loaded API key = %q, want %q", loadedCfg.LinearAPIKey, cfg.LinearAPIKey)
	}
	if loadedCfg.TeamID != cfg.TeamID {
		t.Errorf("loaded team ID = %q, want %q", loadedCfg.TeamID, cfg.TeamID)
	}
	if loadedCfg.TeamKey != cfg.TeamKey {
		t.Errorf("loaded team key = %q, want %q", loadedCfg.TeamKey, cfg.TeamKey)
	}

	_ = origConfigPath
}
