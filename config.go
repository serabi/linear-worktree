package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	if err := validateClaudeCommand(cfg.ClaudeCommand); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func SaveConfig(cfg Config) error {
	if err := validateClaudeCommand(cfg.ClaudeCommand); err != nil {
		return err
	}

	path := configPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

var validCommand = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

func validateClaudeCommand(cmd string) error {
	if !validCommand.MatchString(cmd) {
		return fmt.Errorf("invalid claude_command %q: must contain only alphanumeric characters, dots, slashes, hyphens, and underscores", cmd)
	}
	return nil
}

func (c Config) NeedsSetup() bool {
	return c.LinearAPIKey == "" || c.TeamID == ""
}
