package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
)

func LaunchClaude(wtPath string, issue Issue, cfg Config) error {
	identifier := issue.Identifier
	sessionName := "wt-" + strings.ToLower(identifier)

	prompt := buildPrompt(issue, cfg)

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

func buildPrompt(issue Issue, cfg Config) string {
	if cfg.PromptTemplate != "" {
		tmpl, err := template.New("prompt").Parse(cfg.PromptTemplate)
		if err != nil {
			debugLog.Printf("prompt template parse error: %v", err)
		} else {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, issue); err != nil {
				debugLog.Printf("prompt template exec error: %v", err)
			} else {
				return buf.String()
			}
		}
	}
	prompt := fmt.Sprintf("You're working on %s: %s", issue.Identifier, issue.Title)
	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		prompt += fmt.Sprintf("\n\nDescription:\n%s", desc)
	}
	return prompt
}

func buildShellCmd(prompt string, cfg Config) string {
	parts := []string{cfg.ClaudeCommand}
	if cfg.ClaudeArgs != "" {
		parts = append(parts, cfg.ClaudeArgs)
	}
	if prompt != "" {
		parts = append(parts, shellQuote(prompt))
	}
	return strings.Join(parts, " ")
}

func RunPostCreateHook(wtPath string, cfg Config) error {
	if cfg.PostCreateHook == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", cfg.PostCreateHook)
	cmd.Dir = wtPath
	return cmd.Run()
}

func launchTmux(wtPath, sessionName, prompt string, cfg Config) error {
	shellCmd := buildShellCmd(prompt, cfg)

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
	shellCmd := buildShellCmd(prompt, cfg)

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
