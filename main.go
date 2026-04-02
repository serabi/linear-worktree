package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	flag.Parse()

	var cfg Config
	if *demoMode {
		cfg = DemoConfig()
	} else {
		var err error
		cfg, err = LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
	}
	debugLog.Printf("startup: TeamKey=%q Teams=%v", cfg.TeamKey, cfg.Teams)

	m := NewModel(cfg)
	m.demo = *demoMode
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
