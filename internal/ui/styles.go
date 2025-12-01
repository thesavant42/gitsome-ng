package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants - single source of truth for all viewport dimensions
const (
	MinViewportWidth = 80  // Minimum usable width
	DefaultWidth     = 110 // Used when terminal size is unknown
	TableHeight      = 20
	BorderPadding    = 2 // left/right padding inside borders
)

// Layout holds computed dimensions for the current terminal size
type Layout struct {
	ViewportWidth  int // clamped terminal width
	ViewportHeight int // terminal height
	ContentWidth   int // ViewportWidth - border chars
	TableWidth     int // sum of column widths + separators
	TableHeight    int // available height for table rows
	InnerWidth     int // ViewportWidth - 2 (EXACT width for content inside borders - THE ONE RULE)
}

// NewLayout creates a Layout from the terminal dimensions
func NewLayout(terminalWidth, terminalHeight int) Layout {
	width := terminalWidth
	if width < MinViewportWidth {
		width = MinViewportWidth
	}
	// Calculate table height: terminal height minus overhead
	// Overhead: 1 top margin + 2 border (top/bottom) + 2 padding + 3 tabs + 2 header + 1 footer = ~11 lines
	tableHeight := terminalHeight - 11
	if tableHeight < 5 {
		tableHeight = 5
	}
	return Layout{
		ViewportWidth:  width, // full terminal width for border
		ViewportHeight: terminalHeight,
		ContentWidth:   width - 2,   // content inside border
		TableWidth:     width - 4,   // minus border (2) + minimal table padding (2)
		TableHeight:    tableHeight, // dynamic table height
		InnerWidth:     width - 2,   // ViewportWidth - 2 (content inside borders)
	}
}

// DefaultLayout returns a layout using the default dimensions
func DefaultLayout() Layout {
	return NewLayout(DefaultWidth, 30)
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

// Total returns the sum of all column widths
func (w ColumnWidths) Total() int {
	return w.Tag + w.Rank + w.Name + w.Login + w.Email + w.Commits + w.Percent
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

	// Hint/help text style - yellow for visibility
	HintStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

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

	// Dim text style (gray) - for secondary/muted information
	DimStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// Description style for list item descriptions
	DescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	// Status message style - white on red highlight
	StatusMsgStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorHighlight).
			Padding(0, 1)

	// Progress bar filled style (red)
	ProgressFilledStyle = lipgloss.NewStyle().
				Foreground(ColorBorder)

	// Progress bar empty style (gray)
	ProgressEmptyStyle = lipgloss.NewStyle().
				Foreground(ColorTextDim)
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

// ApplyTableStyles applies the app's standard styling to a table
// This centralizes table styling so we don't use lipgloss directly in other files
func ApplyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorText).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorText)
	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)
	s.Cell = s.Cell.Foreground(ColorText)
	t.SetStyles(s)
}

// NewAppSpinner creates a spinner with the app's standard styling
func NewAppSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorBorder)
	return s
}

// SpinnerStyle is the style for spinner text
var SpinnerStyle = lipgloss.NewStyle().Foreground(ColorBorder)

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
