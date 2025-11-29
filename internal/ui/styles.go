package ui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants - single source of truth for all viewport dimensions
const (
	MinViewportWidth  = 110
	MaxViewportWidth  = 140
	DefaultWidth      = 110 // Used when terminal size is unknown
	TableHeight       = 20
	BorderPadding     = 2 // left/right padding inside borders
)

// Layout holds computed dimensions for the current terminal size
type Layout struct {
	ViewportWidth int // clamped terminal width
	ContentWidth  int // ViewportWidth - border chars
	TableWidth    int // sum of column widths + separators
}

// NewLayout creates a Layout from the terminal width, clamping to min/max
func NewLayout(terminalWidth int) Layout {
	width := clamp(terminalWidth, MinViewportWidth, MaxViewportWidth)
	return Layout{
		ViewportWidth: width,
		ContentWidth:  width - 2,  // minus border chars
		TableWidth:    width - 4,  // minus border + padding
	}
}

// DefaultLayout returns a layout using the default width
func DefaultLayout() Layout {
	return NewLayout(DefaultWidth)
}

// clamp restricts a value to the given range
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// TableColumns defines the main committer table columns - single source of truth
var TableColumns = []table.Column{
	{Title: "Tag", Width: 5},
	{Title: "Rank", Width: 6},
	{Title: "Name", Width: 20},
	{Title: "GitHub Login", Width: 15},
	{Title: "Email", Width: 40},
	{Title: "Commits", Width: 8},
	{Title: "%", Width: 7},
}

// Color palette - centralized color definitions
var (
	ColorBorder       = lipgloss.Color("196") // red
	ColorHighlight    = lipgloss.Color("88")  // dark red background
	ColorText         = lipgloss.Color("15")  // bright white
	ColorAccent       = lipgloss.Color("226") // bright yellow
	ColorAccentDim    = lipgloss.Color("220") // yellow (progress)
	ColorTextDim      = lipgloss.Color("241") // gray
	ColorBlack        = lipgloss.Color("0")   // black
)

// Link colors for link groups
var LinkColors = []lipgloss.Color{
	lipgloss.Color("86"),  // cyan
	lipgloss.Color("226"), // bright yellow
	lipgloss.Color("213"), // magenta
	lipgloss.Color("208"), // orange
	lipgloss.Color("141"), // purple
	lipgloss.Color("220"), // yellow
	lipgloss.Color("39"),  // blue
	lipgloss.Color("196"), // red
}

// Common styles - reusable style definitions
var (
	// Border style for main viewport
	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder)

	// Title style for section headers
	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText).
		MarginBottom(1)

	// Selected row/item style
	SelectedStyle = lipgloss.NewStyle().
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)

	// Normal text style
	NormalStyle = lipgloss.NewStyle().
		Foreground(ColorText)

	// Hint/help text style
	HintStyle = lipgloss.NewStyle().
		Foreground(ColorText).
		Italic(true)

	// Accent style for highlighted text (yellow)
	AccentStyle = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)

	// Progress style
	ProgressStyle = lipgloss.NewStyle().
		Foreground(ColorAccentDim)

	// Tab styles
	TabActiveStyle = lipgloss.NewStyle().
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true).
		Padding(0, 2)

	TabInactiveStyle = lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 2)

	// Arrow style for pagination
	ArrowStyle = lipgloss.NewStyle().
		Foreground(ColorBorder).
		Bold(true)

	// Stats footer style
	StatsStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText)
)

// BorderedBox returns a style for bordered content boxes with the layout width
func BorderedBox(layout Layout) lipgloss.Style {
	return BorderStyle.
		Padding(1, 2).
		Width(layout.ViewportWidth)
}

// BorderedBoxDefault returns a bordered box with default width
func BorderedBoxDefault() lipgloss.Style {
	return BorderedBox(DefaultLayout())
}

