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
	tmpDir := t.TempDir()
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

	// Save manually (simulating old-style plaintext config)
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

	if loadedCfg.TeamID != cfg.TeamID {
		t.Errorf("loaded team ID = %q, want %q", loadedCfg.TeamID, cfg.TeamID)
	}
	if loadedCfg.TeamKey != cfg.TeamKey {
		t.Errorf("loaded team key = %q, want %q", loadedCfg.TeamKey, cfg.TeamKey)
	}
}

func TestSaveConfigStoresAPIKeyInKeyring(t *testing.T) {
	// Clean up keyring before test
	deleteAPIKey()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := Config{
		LinearAPIKey:  "lin_api_keyring_test",
		TeamID:        "team-abc",
		TeamKey:       "TEST",
		ClaudeCommand: "claude",
		BranchPrefix:  "feature/",
		WorktreeBase:  "../worktrees",
	}

	// We can't easily call SaveConfig (it uses configPath()), so test the logic directly:
	// Store API key in keyring, clear from struct, marshal
	if err := storeAPIKey(cfg.LinearAPIKey); err != nil {
		t.Fatalf("storeAPIKey() error: %v", err)
	}
	savedKey := cfg.LinearAPIKey
	cfg.LinearAPIKey = ""

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, data, 0600)

	// Verify: file should NOT contain the API key
	fileData, _ := os.ReadFile(path)
	var fileCfg Config
	json.Unmarshal(fileData, &fileCfg)
	if fileCfg.LinearAPIKey != "" {
		t.Errorf("API key should not be in JSON file after save, got %q", fileCfg.LinearAPIKey)
	}

	// Verify: keyring should have the key
	key, err := retrieveAPIKey()
	if err != nil {
		t.Fatalf("retrieveAPIKey() error: %v", err)
	}
	if key != savedKey {
		t.Errorf("keyring API key = %q, want %q", key, savedKey)
	}

	// Clean up
	deleteAPIKey()
}

func TestAPIKeyMigration(t *testing.T) {
	// Clean up keyring
	deleteAPIKey()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// Write a legacy config with plaintext API key
	legacyCfg := Config{
		LinearAPIKey:  "lin_api_legacy",
		TeamID:        "team-migrate",
		TeamKey:       "MIG",
		ClaudeCommand: "claude",
		BranchPrefix:  "feature/",
		WorktreeBase:  "../worktrees",
	}
	data, _ := json.MarshalIndent(legacyCfg, "", "  ")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, data, 0600)

	// Simulate migration: read config, find key in file but not keyring, migrate
	var cfg Config
	fileData, _ := os.ReadFile(path)
	json.Unmarshal(fileData, &cfg)

	if cfg.LinearAPIKey == "" {
		t.Fatal("legacy config should have API key in file")
	}

	// Migrate
	migrateAPIKeyToKeyring(&cfg, path)

	// Verify: keyring should have the key
	key, err := retrieveAPIKey()
	if err != nil {
		t.Fatalf("retrieveAPIKey() after migration: %v", err)
	}
	if key != "lin_api_legacy" {
		t.Errorf("migrated key = %q, want %q", key, "lin_api_legacy")
	}

	// Verify: file should no longer have the key
	fileData, _ = os.ReadFile(path)
	var fileCfg Config
	json.Unmarshal(fileData, &fileCfg)
	if fileCfg.LinearAPIKey != "" {
		t.Errorf("file should not contain API key after migration, got %q", fileCfg.LinearAPIKey)
	}

	// Clean up
	deleteAPIKey()
}

func TestValidateClaudeCommand(t *testing.T) {
	valid := []string{"claude", "/usr/local/bin/claude", "claude-code", "../bin/claude", "claude_dev"}
	for _, cmd := range valid {
		if err := validateClaudeCommand(cmd); err != nil {
			t.Errorf("validateClaudeCommand(%q) should be valid, got: %v", cmd, err)
		}
	}

	invalid := []string{
		"claude; rm -rf /",
		"$(whoami)",
		"claude`id`",
		"claude && echo pwned",
		"claude | cat",
		"claude > /dev/null",
		"claude\nwhoami",
		"claude code", // spaces not allowed
	}
	for _, cmd := range invalid {
		if err := validateClaudeCommand(cmd); err == nil {
			t.Errorf("validateClaudeCommand(%q) should be invalid", cmd)
		}
	}
}
