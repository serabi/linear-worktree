package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type TeamEntry struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type Config struct {
	LinearAPIKey  string   `json:"linear_api_key,omitempty"`
	TeamID        string   `json:"team_id"`
	TeamKey       string   `json:"team_key"`
	Teams         []TeamEntry `json:"teams,omitempty"`
	WorktreeBase  string   `json:"worktree_base_dir"`
	CopyFiles     []string `json:"copy_files"`
	CopyDirs      []string `json:"copy_dirs"`
	ClaudeCommand  string `json:"claude_command"`
	ClaudeArgs     string `json:"claude_args"`
	BranchPrefix   string `json:"branch_prefix"`
	MaxSlots       int    `json:"max_slots"` // 2, 3, or 4
	PostCreateHook string `json:"post_create_hook"`
	PromptTemplate string `json:"prompt_template"`
	SlotColors     []string `json:"slot_colors,omitempty"` // palette name per slot (indexed 0..3); see slotPaletteNames
}

// slotPaletteNames lists the palette entries accepted in Config.SlotColors.
// Values map to adaptive colors via slotPaletteColor in model_theme.go.
var slotPaletteNames = []string{"green", "blue", "purple", "orange", "pink", "cyan", "yellow", "red"}

// defaultSlotColors returns the default per-slot palette assignment.
var defaultSlotColors = []string{"green", "blue", "purple", "orange"}

// SlotColorName returns the palette name configured for the given slot index,
// falling back to the default assignment. The returned name is always a valid
// palette entry.
func (c Config) SlotColorName(idx int) string {
	if idx >= 0 && idx < len(c.SlotColors) {
		name := c.SlotColors[idx]
		for _, valid := range slotPaletteNames {
			if name == valid {
				return name
			}
		}
	}
	if idx >= 0 && idx < len(defaultSlotColors) {
		return defaultSlotColors[idx]
	}
	return "green"
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

	// Migrate legacy single-team config to Teams slice
	if len(cfg.Teams) == 0 && cfg.TeamID != "" && cfg.TeamKey != "" {
		cfg.Teams = []TeamEntry{{ID: cfg.TeamID, Key: cfg.TeamKey}}
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
	return c.LinearAPIKey == "" || (c.TeamID == "" && len(c.Teams) == 0)
}
