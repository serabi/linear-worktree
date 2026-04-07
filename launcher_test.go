package main

import "testing"

func TestBuildShellCmd(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		prompt string
		want   string
	}{
		{
			name:   "no args no prompt",
			cfg:    Config{ClaudeCommand: "claude"},
			prompt: "",
			want:   "claude",
		},
		{
			name:   "no args with prompt",
			cfg:    Config{ClaudeCommand: "claude"},
			prompt: "hello",
			want:   "claude 'hello'",
		},
		{
			name:   "simple args",
			cfg:    Config{ClaudeCommand: "claude", ClaudeArgs: "--model sonnet"},
			prompt: "hi",
			want:   "claude '--model' 'sonnet' 'hi'",
		},
		{
			name:   "arg with quoted spaces",
			cfg:    Config{ClaudeCommand: "claude", ClaudeArgs: `--system-prompt "be concise"`},
			prompt: "hi",
			want:   "claude '--system-prompt' 'be concise' 'hi'",
		},
		{
			name:   "arg containing single quote via double quotes",
			cfg:    Config{ClaudeCommand: "claude", ClaudeArgs: `--flag "it's fine"`},
			prompt: "",
			want:   `claude '--flag' 'it'\''s fine'`,
		},
		{
			name:   "arg with special shell chars",
			cfg:    Config{ClaudeCommand: "claude", ClaudeArgs: `--opt "$HOME; rm -rf /"`},
			prompt: "",
			want:   `claude '--opt' '$HOME; rm -rf /'`,
		},
		{
			name:   "unterminated quote falls back to fields",
			cfg:    Config{ClaudeCommand: "claude", ClaudeArgs: `--bad "unterminated`},
			prompt: "",
			want:   `claude '--bad' '"unterminated'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildShellCmd(tt.prompt, tt.cfg)
			if got != tt.want {
				t.Errorf("buildShellCmd() = %q, want %q", got, tt.want)
			}
		})
	}
}
