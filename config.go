package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Config struct {
	LinearAPIKey  string   `json:"linear_api_key,omitempty"`
	TeamID        string   `json:"team_id"`
	TeamKey       string   `json:"team_key"`
	WorktreeBase  string   `json:"worktree_base_dir"`
	CopyFiles     []string `json:"copy_files"`
	CopyDirs      []string `json:"copy_dirs"`
	ClaudeCommand string   `json:"claude_command"`
	BranchPrefix  string   `json:"branch_prefix"`
	MaxSlots      int      `json:"max_slots"` // 2, 3, or 4
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

	// Resolve API key: keychain → legacy JSON → env var
	if key, err := retrieveAPIKey(); err == nil {
		cfg.LinearAPIKey = key
	}

	// Migrate legacy plaintext key to keychain
	if cfg.LinearAPIKey != "" {
		if _, err := retrieveAPIKey(); isKeyringNotFound(err) {
			if err := migrateAPIKeyToKeyring(&cfg, path); err != nil {
				fmt.Fprintf(os.Stderr, "api key migration failed for %s: %v\n", path, err)
				return cfg, err
			}
		}
	}

	// Env var fallback
	if cfg.LinearAPIKey == "" {
		cfg.LinearAPIKey = os.Getenv("LINEAR_API_KEY")
	}

	if err := validateClaudeCommand(cfg.ClaudeCommand); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// migrateAPIKeyToKeyring moves a plaintext API key from the config file to the
// OS keychain and rewrites the config without the key.
func migrateAPIKeyToKeyring(cfg *Config, path string) error {
	if err := storeAPIKey(cfg.LinearAPIKey); err != nil {
		return nil // keychain unavailable, keep in file
	}
	// Rewrite config file without the API key
	fileCfg := *cfg
	fileCfg.LinearAPIKey = ""
	if data, err := json.MarshalIndent(fileCfg, "", "  "); err == nil {
		if err := os.WriteFile(path, data, 0600); err != nil {
			return fmt.Errorf("failed to rewrite migrated config at %s: %w", path, err)
		}
	}
	return nil
}

func SaveConfig(cfg Config) error {
	if err := validateClaudeCommand(cfg.ClaudeCommand); err != nil {
		return err
	}

	// Store API key in keychain
	if cfg.LinearAPIKey != "" {
		if err := storeAPIKey(cfg.LinearAPIKey); err == nil {
			cfg.LinearAPIKey = "" // don't write to file
		}
		// If keychain fails, the key stays in the struct and gets written to file
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
