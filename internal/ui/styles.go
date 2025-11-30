package ui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants - single source of truth for all viewport dimensions
const (
	MinViewportWidth = 110
	MaxViewportWidth = 140
	DefaultWidth     = 110 // Used when terminal size is unknown
	TableHeight      = 20
	BorderPadding    = 2 // left/right padding inside borders
)

// Layout holds computed dimensions for the current terminal size
type Layout struct {
	ViewportWidth int // clamped terminal width
	ContentWidth  int // ViewportWidth - border chars
	TableWidth    int // sum of column widths + separators
	InnerWidth    int // ViewportWidth - 2 (EXACT width for content inside borders - THE ONE RULE)
}

// NewLayout creates a Layout from the terminal width, clamping to min/max
func NewLayout(terminalWidth int) Layout {
	width := clamp(terminalWidth, MinViewportWidth, MaxViewportWidth)
	return Layout{
		ViewportWidth: width,
		ContentWidth:  width - 2, // minus border chars
		TableWidth:    width - 4, // minus border + padding
		InnerWidth:    width - 0, // EXACT width for content inside borders (THE ONE RULE)
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

// Minimum column widths (used as fallback and for header sizing)
const (
	ColWidthTag     = 5
	ColWidthRank    = 6
	ColWidthName    = 12
	ColWidthLogin   = 12
	ColWidthEmail   = 20
	ColWidthCommits = 8
	ColWidthPercent = 7
	// 6 column separators at 2 spaces each = 12
	ColSeparators = 12
)

// ColumnWidths holds the calculated widths for each column
type ColumnWidths struct {
	Tag     int
	Rank    int
	Name    int
	Login   int
	Email   int
	Commits int
	Percent int
}

// DefaultColumnWidths returns minimum column widths
func DefaultColumnWidths() ColumnWidths {
	return ColumnWidths{
		Tag:     ColWidthTag,
		Rank:    ColWidthRank,
		Name:    ColWidthName,
		Login:   ColWidthLogin,
		Email:   ColWidthEmail,
		Commits: ColWidthCommits,
		Percent: ColWidthPercent,
	}
}

// BuildTableColumns creates table columns from calculated widths
func BuildTableColumns(widths ColumnWidths) []table.Column {
	return []table.Column{
		{Title: "Tag", Width: widths.Tag},
		{Title: "Rank", Width: widths.Rank},
		{Title: "Name", Width: widths.Name},
		{Title: "GitHub Login", Width: widths.Login},
		{Title: "Email", Width: widths.Email},
		{Title: "Commits", Width: widths.Commits},
		{Title: "%", Width: widths.Percent},
	}
}

// Color palette - centralized color definitions
var (
	ColorBorder    = lipgloss.Color("196") // red
	ColorHighlight = lipgloss.Color("88")  // dark red background
	ColorText      = lipgloss.Color("15")  // bright white
	ColorAccent    = lipgloss.Color("226") // bright yellow
	ColorAccentDim = lipgloss.Color("220") // yellow (progress)
	ColorTextDim   = lipgloss.Color("241") // gray
	ColorBlack     = lipgloss.Color("0")   // black
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
	// STYLE GUIDE: Always use .Width(ViewportWidth) with NO .Padding()
	// This ensures consistent 2-char overhead (left + right border edges)
	// Content inside borders must use InnerWidth (ViewportWidth - 2)
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

	// Gist divider row style - accent yellow, no background
	DividerStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	// Gist divider row selected style - accent on red highlight
	DividerSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Background(ColorHighlight).
				Bold(true)
)

// BorderedBox returns a style for bordered content boxes with the layout width
func BorderedBox(layout Layout) lipgloss.Style {
	return BorderStyle.
		Padding(1, 0).
		Width(layout.ViewportWidth)
}

// BorderedBoxDefault returns a bordered box with default width
func BorderedBoxDefault() lipgloss.Style {
	return BorderedBox(DefaultLayout())
}

// NewAppTheme creates a huh theme matching the app's style guide
// White text, red highlights/selection
func NewAppTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Title styling - white bold
	t.Focused.Title = lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true)
	t.Blurred.Title = t.Focused.Title

	// Description - white
	t.Focused.Description = lipgloss.NewStyle().
		Foreground(ColorText)
	t.Blurred.Description = t.Focused.Description

	// Base text - white
	t.Focused.Base = lipgloss.NewStyle().
		Foreground(ColorText)
	t.Blurred.Base = t.Focused.Base

	// Selected option - red background, white text
	t.Focused.SelectedOption = lipgloss.NewStyle().
		Foreground(ColorText).
		Background(ColorBorder).
		Bold(true).
		Padding(0, 1)

	// Unselected option - white text
	t.Focused.UnselectedOption = lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 1)

	// Focus bar (the | indicator) - red
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Foreground(ColorText).
		Background(ColorBorder).
		Bold(true).
		Padding(0, 1)

	t.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 1)

	// Text input styling
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(ColorBorder)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(ColorTextDim)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().
		Foreground(ColorBorder)

	return t
}
