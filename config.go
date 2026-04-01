package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	LinearAPIKey   string   `json:"linear_api_key"`
	TeamID         string   `json:"team_id"`
	TeamKey        string   `json:"team_key"`
	WorktreeBase   string   `json:"worktree_base_dir"`
	CopyFiles      []string `json:"copy_files"`
	CopyDirs       []string `json:"copy_dirs"`
	ClaudeCommand  string   `json:"claude_command"`
	BranchPrefix   string   `json:"branch_prefix"`
	MaxSlots       int      `json:"max_slots"` // 2, 3, or 4
}

func DefaultConfig() Config {
	return Config{
		WorktreeBase:  "../worktrees",
		CopyFiles:     []string{".env", ".envrc"},
		CopyDirs:      []string{".claude"},
		ClaudeCommand: "claude",
		BranchPrefix:  "feature/",
		MaxSlots:      3,
	}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "linear-worktree", "config.json")
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	// Fill defaults for empty fields
	if cfg.ClaudeCommand == "" {
		cfg.ClaudeCommand = "claude"
	}
	if cfg.BranchPrefix == "" {
		cfg.BranchPrefix = "feature/"
	}
	if cfg.WorktreeBase == "" {
		cfg.WorktreeBase = "../worktrees"
	}

	if cfg.MaxSlots < 2 || cfg.MaxSlots > 4 {
		cfg.MaxSlots = 3
	}

	// Also check env var
	if cfg.LinearAPIKey == "" {
		cfg.LinearAPIKey = os.Getenv("LINEAR_API_KEY")
	}

	return cfg, nil
}

func SaveConfig(cfg Config) error {
	path := configPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c Config) NeedsSetup() bool {
	return c.LinearAPIKey == "" || c.TeamID == ""
}
