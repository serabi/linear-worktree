package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func LaunchClaude(wtPath string, issue Issue, cfg Config) error {
	identifier := issue.Identifier
	sessionName := "wt-" + strings.ToLower(identifier)

	// Build prompt with issue context
	prompt := fmt.Sprintf("You're working on %s: %s", identifier, issue.Title)
	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		prompt += fmt.Sprintf("\n\nDescription:\n%s", desc)
	}

	// Check if tmux session already exists
	check := exec.Command("tmux", "has-session", "-t", sessionName)
	if check.Run() == nil {
		// Session exists, switch to it
		return exec.Command("tmux", "switch-client", "-t", sessionName).Run()
	}

	// Try cmux first
	if cmuxPath, err := exec.LookPath("cmux"); err == nil {
		return launchCmux(cmuxPath, wtPath, sessionName, prompt, cfg)
	}

	// Fall back to tmux
	return launchTmux(wtPath, sessionName, prompt, cfg)
}

func LaunchClaudeWithPrompt(wtPath string, issue Issue, prompt string, cfg Config) error {
	sessionName := "wt-" + strings.ToLower(issue.Identifier)

	check := exec.Command("tmux", "has-session", "-t", sessionName)
	if check.Run() == nil {
		return exec.Command("tmux", "switch-client", "-t", sessionName).Run()
	}

	if cmuxPath, err := exec.LookPath("cmux"); err == nil {
		return launchCmux(cmuxPath, wtPath, sessionName, prompt, cfg)
	}

	return launchTmux(wtPath, sessionName, prompt, cfg)
}

func launchTmux(wtPath, sessionName, prompt string, cfg Config) error {
	var shellCmd string
	if prompt != "" {
		shellCmd = fmt.Sprintf("%s %s", cfg.ClaudeCommand, shellQuote(prompt))
	} else {
		shellCmd = cfg.ClaudeCommand
	}

	cmd := exec.Command(
		"tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-c", wtPath,
		shellCmd,
	)
	return cmd.Run()
}

func launchCmux(cmuxPath, wtPath, sessionName, prompt string, cfg Config) error {
	var shellCmd string
	if prompt != "" {
		shellCmd = fmt.Sprintf("%s %s", cfg.ClaudeCommand, shellQuote(prompt))
	} else {
		shellCmd = cfg.ClaudeCommand
	}

	cmd := exec.Command(
		cmuxPath, "workspace", "create",
		"--name", sessionName,
		"--cwd", wtPath,
		"--command", shellCmd,
	)

	if err := cmd.Run(); err != nil {
		// Fall back to tmux on cmux failure
		return launchTmux(wtPath, sessionName, prompt, cfg)
	}
	return nil
}
