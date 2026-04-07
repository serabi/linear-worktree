package main

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	// Adaptive colors for light/dark terminal themes.
	// AdaptiveColor: {Light, Dark}
	// Adaptive colors for WCAG AA contrast on both light and dark backgrounds.
	// Light values target >= 4.5:1 against #F5F5F5; dark values target visibility on #1E1E1E.
	dimColor       = compat.AdaptiveColor{Light: lipgloss.Color("#444"), Dark: lipgloss.Color("#888")}
	subtleColor    = compat.AdaptiveColor{Light: lipgloss.Color("#555"), Dark: lipgloss.Color("#555")}
	mutedColor     = compat.AdaptiveColor{Light: lipgloss.Color("#555"), Dark: lipgloss.Color("#666")}
	faintColor     = compat.AdaptiveColor{Light: lipgloss.Color("#646464"), Dark: lipgloss.Color("#444")}
	yellowColor    = compat.AdaptiveColor{Light: lipgloss.Color("#B45309"), Dark: lipgloss.Color("#EAB308")}
	identCyanColor = compat.AdaptiveColor{Light: lipgloss.Color("#0E7490"), Dark: lipgloss.Color("#06B6D4")}
	greenColor     = compat.AdaptiveColor{Light: lipgloss.Color("#15803D"), Dark: lipgloss.Color("#22C55E")}
	redColor       = compat.AdaptiveColor{Light: lipgloss.Color("#B91C1C"), Dark: lipgloss.Color("#EF4444")}
	orangeColor    = compat.AdaptiveColor{Light: lipgloss.Color("#C2410C"), Dark: lipgloss.Color("#F97316")}
	blueColor      = compat.AdaptiveColor{Light: lipgloss.Color("#2563EB"), Dark: lipgloss.Color("#3B82F6")}

	appStyle = lipgloss.NewStyle().Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Padding(0, 1)

	issueIdentStyle = lipgloss.NewStyle().
			Foreground(identCyanColor).
			Bold(true)

	worktreeMarker = lipgloss.NewStyle().
			Foreground(greenColor)

	urgentStyle = lipgloss.NewStyle().Foreground(redColor)
	highStyle   = lipgloss.NewStyle().Foreground(orangeColor)
	mediumStyle = lipgloss.NewStyle().Foreground(yellowColor)
	lowStyle    = lipgloss.NewStyle().Foreground(blueColor)

	setupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Width(50)

	slotWaitingStyle = lipgloss.NewStyle().Foreground(yellowColor)
	slotEmptyStyle   = lipgloss.NewStyle().Foreground(faintColor)

	// slotPaletteColors maps palette names (see slotPaletteNames in config.go)
	// to adaptive colors. Light variants target WCAG AA on #F5F5F5; dark
	// variants target visibility on #1E1E1E.
	slotPaletteColors = map[string]compat.AdaptiveColor{
		"green":  greenColor,
		"blue":   blueColor,
		"purple": {Light: lipgloss.Color("#7C3AED"), Dark: lipgloss.Color("#A78BFA")},
		"orange": orangeColor,
		"pink":   {Light: lipgloss.Color("#BE185D"), Dark: lipgloss.Color("#EC4899")},
		"cyan":   identCyanColor,
		"yellow": yellowColor,
		"red":    redColor,
	}

	commentDimStyle = lipgloss.NewStyle().Foreground(dimColor)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(dimColor).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(faintColor).
				Padding(0, 2)
)

// slotPaletteColor resolves a palette name to an AdaptiveColor. Unknown
// names fall back to green.
func slotPaletteColor(name string) compat.AdaptiveColor {
	if c, ok := slotPaletteColors[name]; ok {
		return c
	}
	return slotPaletteColors["green"]
}

// slotBadgeStyle returns the lipgloss style to render the status badge for the
// given palette name and status. The badge is colored using the per-slot
// palette (so different slots are visually distinct) except when the slot is
// waiting on input -- that case forces the yellow waiting style so users
// don't miss prompts.
func slotBadgeStyle(paletteName string, status AgentStatus) lipgloss.Style {
	if status == AgentWaiting {
		return slotWaitingStyle
	}
	if status == AgentInactive {
		return slotEmptyStyle
	}
	s := lipgloss.NewStyle().Foreground(slotPaletteColor(paletteName))
	if status == AgentIdle {
		s = s.Faint(true)
	}
	return s
}
