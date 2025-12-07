package ui

// PURE ANSI STYLING - NO LIPGLOSS
// All styling uses ANSI escape codes directly.
// This file provides the ONLY styling definitions for the entire UI.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/huh"
	"github.com/mattn/go-runewidth"
)

// =============================================================================
// ANSI Escape Code Constants
// =============================================================================

const (
	// Reset all attributes
	ansiReset = "\033[0m"

	// Text attributes
	ansiBold   = "\033[1m"
	ansiItalic = "\033[3m"

	// 256-color foreground: \033[38;5;{n}m
	// 256-color background: \033[48;5;{n}m
)

// ANSI 256-color codes
const (
	colorRed       = 196 // red (border, accent)
	colorDarkRed   = 88  // dark red (highlight/selection background)
	colorWhite     = 15  // bright white (text)
	colorYellow    = 226 // bright yellow (accent)
	colorYellowDim = 220 // yellow (progress)
	colorGray      = 241 // gray (dim text)
	colorBlack     = 0   // black

	// Row status colors
	colorPending = 228 // Light yellow - pending selection
	colorLinked  = 118 // Green - linked rows
	colorDomain  = 208 // Orange - domain rows

	// Report colors
	colorPurple = 99  // Purple
	colorPink   = 205 // Pink
	colorGreen  = 46  // Green
	colorCyan   = 51  // Cyan
	colorWhite2 = 255 // White

	// Link group colors
	colorCyanLink    = 86
	colorYellowLink  = 226
	colorMagentaLink = 213
	colorOrangeLink  = 208
	colorPurpleLink  = 141
	colorYellow2Link = 220
	colorBlueLink    = 39
	colorRedLink     = 196
)

// Color string exports for progress bar and other components
// These are used as: string(ColorText) -> "#FFFFFF" style notation
// Using ANSI 256-color indices as hex approximations for Charm components
const (
	ColorText    = "15"  // bright white (ANSI 256 color 15)
	ColorTextDim = "241" // gray (ANSI 256 color 241)
)

// LinkColors is an alias for LinkGroupColors (used by tui.go)
var LinkColors = LinkGroupColors

// LinkGroupColors contains the color codes for link groups
var LinkGroupColors = []int{
	colorCyanLink,
	colorYellowLink,
	colorMagentaLink,
	colorOrangeLink,
	colorPurpleLink,
	colorYellow2Link,
	colorBlueLink,
	colorRedLink,
}

// =============================================================================
// ANSI Styling Functions
// =============================================================================

// fg returns ANSI foreground color escape sequence
func fg(color int) string {
	return fmt.Sprintf("\033[38;5;%dm", color)
}

// bg returns ANSI background color escape sequence
func bg(color int) string {
	return fmt.Sprintf("\033[48;5;%dm", color)
}

// style applies foreground color and optional attributes to text
func style(text string, fgColor int, bold bool) string {
	if bold {
		return fg(fgColor) + ansiBold + text + ansiReset
	}
	return fg(fgColor) + text + ansiReset
}

// styleWithBg applies foreground, background colors and optional attributes
func styleWithBg(text string, fgColor, bgColor int, bold bool) string {
	if bold {
		return fg(fgColor) + bg(bgColor) + ansiBold + text + ansiReset
	}
	return fg(fgColor) + bg(bgColor) + text + ansiReset
}

// padRight pads a string to a specific width (for full-width styling)
func padRight(s string, width int) string {
	visibleLen := StringWidth(s)
	if visibleLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleLen)
}

// =============================================================================
// Style Rendering Functions
// =============================================================================

// RenderTitle renders text as a title (bold white)
func RenderTitle(text string) string {
	return style(text, colorWhite, true)
}

// RenderNormal renders text in normal style (white)
func RenderNormal(text string) string {
	return style(text, colorWhite, false)
}

// RenderSelected renders text with selection highlight (white on dark red, bold)
func RenderSelected(text string) string {
	return styleWithBg(text, colorWhite, colorDarkRed, true)
}

// RenderSelectedWidth renders text with selection highlight at specific width
func RenderSelectedWidth(text string, width int) string {
	padded := padRight(text, width)
	return styleWithBg(padded, colorWhite, colorDarkRed, true)
}

// RenderHint renders hint/help text (yellow)
func RenderHint(text string) string {
	return style(text, colorYellow, false)
}

// RenderAccent renders accented text (yellow bold)
func RenderAccent(text string) string {
	return style(text, colorYellow, true)
}

// RenderProgress renders progress text (dim yellow)
func RenderProgress(text string) string {
	return style(text, colorYellowDim, false)
}

// RenderDim renders dimmed text (gray)
func RenderDim(text string) string {
	return style(text, colorGray, false)
}

// RenderError renders error text (red bold)
func RenderError(text string) string {
	return style(text, colorRed, true)
}

// RenderSuccess renders success text (green bold)
func RenderSuccess(text string) string {
	return style(text, colorGreen, true)
}

// RenderStats renders stats text (white bold)
func RenderStats(text string) string {
	return style(text, colorWhite, true)
}

// RenderDivider renders divider text (yellow)
func RenderDivider(text string) string {
	return style(text, colorYellow, false)
}

// RenderDividerSelected renders selected divider (yellow on dark red, bold)
func RenderDividerSelected(text string) string {
	return styleWithBg(text, colorYellow, colorDarkRed, true)
}

// RenderStatusMsg renders status message (white on dark red)
func RenderStatusMsg(text string) string {
	return styleWithBg(" "+text+" ", colorWhite, colorDarkRed, false)
}

// RenderTabActive renders active tab (white on dark red, bold)
func RenderTabActive(text string) string {
	return styleWithBg("  "+text+"  ", colorWhite, colorDarkRed, true)
}

// RenderTabInactive renders inactive tab (white)
func RenderTabInactive(text string) string {
	return "  " + style(text, colorWhite, false) + "  "
}

// RenderArrow renders arrow (red bold)
func RenderArrow(text string) string {
	return style(text, colorRed, true)
}

// =============================================================================
// Row Rendering Functions (for tables)
// =============================================================================

// RenderPendingRow renders a row with pending selection styling
func RenderPendingRow(row string, width int) string {
	padded := padRight(row, width)
	return styleWithBg(padded, colorBlack, colorYellowDim, false)
}

// RenderLinkedRow renders a row with link group coloring (full width)
func RenderLinkedRow(row string, groupID int, width int) string {
	colorIdx := (groupID - 1) % len(LinkGroupColors)
	color := LinkGroupColors[colorIdx]
	padded := padRight(row, width)
	return style(padded, color, false)
}

// RenderDomainRow renders a row with domain highlight coloring (yellow, full width)
func RenderDomainRow(row string, width int) string {
	padded := padRight(row, width)
	return style(padded, colorYellow, false)
}

// RenderNormalRow renders a row with normal text coloring (white, full width)
func RenderNormalRow(row string, width int) string {
	padded := padRight(row, width)
	return style(padded, colorWhite, false)
}

// RenderSelectedRow renders a row with selection highlighting (full width)
func RenderSelectedRow(row string, width int) string {
	return RenderSelectedWidth(row, width)
}

// RenderNormalRowWithWidth renders a row with normal text at specific width
func RenderNormalRowWithWidth(row string, width int) string {
	padded := padRight(row, width)
	return style(padded, colorWhite, false)
}

// =============================================================================
// Border Rendering
// =============================================================================

// BorderChars defines the characters for rounded borders
var BorderChars = struct {
	TopLeft, TopRight, BottomLeft, BottomRight string
	Horizontal, Vertical                       string
}{
	TopLeft:     "╭",
	TopRight:    "╮",
	BottomLeft:  "╰",
	BottomRight: "╯",
	Horizontal:  "─",
	Vertical:    "│",
}

// RenderBorder renders content inside a colored border
func RenderBorder(content string, width, height int) string {
	return renderBorderWithColor(content, width, height, colorRed)
}

// RenderBorderWhite renders content inside a white border
func RenderBorderWhite(content string, width, height int) string {
	return renderBorderWithColor(content, width, height, colorWhite)
}

// renderBorderWithColor renders content inside a border with specified color
func renderBorderWithColor(content string, width, height int, borderColor int) string {
	lines := strings.Split(content, "\n")

	// Build top border
	topBorder := style(BorderChars.TopLeft+strings.Repeat(BorderChars.Horizontal, width)+BorderChars.TopRight, borderColor, false)

	// Build content lines with side borders
	var contentLines []string
	for i := 0; i < height; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		// Pad line to width
		visibleLen := StringWidth(line)
		if visibleLen < width {
			line = line + strings.Repeat(" ", width-visibleLen)
		} else if visibleLen > width {
			// Truncate if too long
			line = truncateToWidth(line, width)
		}
		contentLines = append(contentLines, style(BorderChars.Vertical, borderColor, false)+line+style(BorderChars.Vertical, borderColor, false))
	}

	// Build bottom border
	bottomBorder := style(BorderChars.BottomLeft+strings.Repeat(BorderChars.Horizontal, width)+BorderChars.BottomRight, borderColor, false)

	// Combine all parts
	result := topBorder + "\n"
	result += strings.Join(contentLines, "\n") + "\n"
	result += bottomBorder

	return result
}

// truncateToWidth truncates a string to fit within a given visible width
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	currentWidth := 0
	var result strings.Builder
	inEscape := false

	for _, r := range s {
		if r == '\033' {
			inEscape = true
			result.WriteRune(r)
			continue
		}
		if inEscape {
			result.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		runeWidth := runewidth.RuneWidth(r)
		if currentWidth+runeWidth > width {
			break
		}
		result.WriteRune(r)
		currentWidth += runeWidth
	}

	// Add any trailing escape sequences (reset)
	if strings.Contains(s, ansiReset) && !strings.HasSuffix(result.String(), ansiReset) {
		result.WriteString(ansiReset)
	}

	return result.String()
}

// =============================================================================
// Layout (unchanged from original)
// =============================================================================

const (
	MinViewportWidth = 80
	DefaultWidth     = 110
	TableHeight      = 20
	BorderPadding    = 2
)

type Layout struct {
	ViewportWidth  int
	ViewportHeight int
	ContentWidth   int
	TableWidth     int
	TableHeight    int
	InnerWidth     int
}

func NewLayout(terminalWidth, terminalHeight int) Layout {
	const (
		// Content inside main box (before table)
		titleLine          = 1
		newlineAfterTitle  = 1
		separatorLine      = 1
		blankLinesAfterSep = 2
		queryInfoLine      = 1
		blankLineAfterInfo = 1
		contentBeforeTable = titleLine + newlineAfterTitle + separatorLine + blankLinesAfterSep + queryInfoLine + blankLineAfterInfo // = 6

		// Box structure outside main content
		mainBoxBorders  = 2 // top + bottom border
		footerBoxHeight = 3 // 1 content + 2 borders
		boxSpacing      = 1 // newline between main and footer

		minTableHeight = 5 // minimum table height
		borderWidth    = 2 // left + right border

		// bubbles/table SetHeight includes header chrome (header row + BorderBottom line + divider)
		// plus adjustment for data row display
		tableRenderMargin = 4
	)

	width := terminalWidth
	if width < MinViewportWidth {
		width = MinViewportWidth
	}

	// TableHeight = terminal - main borders - content before table - spacing - footer box + table render margin
	tableHeight := terminalHeight - mainBoxBorders - contentBeforeTable - boxSpacing - footerBoxHeight + tableRenderMargin
	if tableHeight < minTableHeight {
		tableHeight = minTableHeight
	}

	return Layout{
		ViewportWidth:  width,
		ViewportHeight: terminalHeight,
		ContentWidth:   width - borderWidth,
		TableWidth:     width - borderWidth,
		TableHeight:    tableHeight,
		InnerWidth:     width - borderWidth,
	}
}

func DefaultLayout() Layout {
	return NewLayout(DefaultWidth, 30)
}

// =============================================================================
// Layout Height Constants - SINGLE SOURCE OF TRUTH
// All View() functions MUST use these via Layout methods.
// =============================================================================

const (
	// MainBoxBorderOverhead is the vertical space taken by main box borders.
	MainBoxBorderOverhead = 2

	// FooterBoxTotalHeight includes border (2) and content (1).
	FooterBoxTotalHeight = 3

	// BoxSpacing is vertical space between main and footer boxes.
	BoxSpacing = 1

	// TwoBoxOverhead is total overhead for two-box layout.
	// MainBoxBorderOverhead + FooterBoxTotalHeight + BoxSpacing = 6
	TwoBoxOverhead = MainBoxBorderOverhead + FooterBoxTotalHeight + BoxSpacing

	// TabbedTableExtraLines is additional content lines in tabbed tables
	// Title (1) + Tab line (1) + Divider (1) + Spacing (2) = 5
	// This is the overhead BEFORE the table, not including table's own header
	TabbedTableExtraLines = 5

	// MinMainContentHeight is the minimum height for main content area.
	MinMainContentHeight = 10

	// MinTableHeight is minimum rows for any table
	MinTableHeight = 5
)

// MainContentHeight returns available height for main box content.
// USE THIS instead of hardcoding ViewportHeight - 6.
func (l Layout) MainContentHeight() int {
	h := l.ViewportHeight - TwoBoxOverhead
	if h < MinMainContentHeight {
		return MinMainContentHeight
	}
	return h
}

// TabbedTableHeight returns table height adjusted for tabbed table chrome.
// USE THIS for all tabbed table views.
func (l Layout) TabbedTableHeight() int {
	h := l.TableHeight - TabbedTableExtraLines
	if h < MinTableHeight {
		return MinTableHeight
	}
	return h
}

// =============================================================================
// Column Widths (unchanged)
// =============================================================================

const (
	ColWidthTag     = 5
	ColWidthRank    = 6
	ColWidthName    = 12
	ColWidthLogin   = 12
	ColWidthEmail   = 20
	ColWidthCommits = 8
	ColWidthPercent = 7
	ColSeparators   = 12
)

type ColumnWidths struct {
	Tag     int
	Rank    int
	Name    int
	Login   int
	Email   int
	Commits int
	Percent int
}

func (w ColumnWidths) Total() int {
	return w.Tag + w.Rank + w.Name + w.Login + w.Email + w.Commits + w.Percent
}

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

// =============================================================================
// Table Styling (ANSI-based)
// =============================================================================

// ApplyTableStyles applies styling to a bubbles table
// Note: bubbles/table still uses its own internal styling, we just configure it
func ApplyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	// Header: bold white with bottom border, NO padding
	s.Header = s.Header.
		BorderBottom(true).
		Bold(true).
		Padding(0)
	// Selected: CRITICAL - must have NO background or it interferes with manual selection
	// Bubbles table has pink default - we MUST unset it completely
	s.Selected = s.Selected.
		Bold(false).
		UnsetBackground().
		UnsetForeground()
	// Cell: no extra padding, clear cell foreground
	s.Cell = s.Cell.UnsetForeground().Padding(0)
	t.SetStyles(s)
}

// GetMainTableStyles returns table styles for the main committer table
func GetMainTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderBottom(true).
		Bold(true)
	// Selected: CRITICAL - must have NO background or it interferes with manual selection
	// Bubbles table has pink default, we strip it in renderTableWithLinks via stripANSI
	s.Selected = s.Selected.Bold(false)
	// Cell: clear cell styling
	s.Cell = s.Cell.UnsetForeground()
	return s
}

// =============================================================================
// Spinner (minimal styling)
// =============================================================================

// SpinnerStyle is a no-op placeholder for Charm component compatibility
// Note: Charm's spinner.Model.Style field requires a specific Style type
// We cannot assign our custom types to it, so spinner styling is left as default
// This variable exists only to prevent compilation errors in code that references it
var SpinnerStyle = styleRenderer{render: func(s string) string { return style(s, colorRed, false) }}

// NewAppSpinner creates a spinner with basic styling
func NewAppSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	// Note: spinner.Style requires a specific type - we cannot assign our custom types
	// The spinner will use default coloring
	return s
}

// =============================================================================
// Huh Theme (for forms)
// =============================================================================

// NewAppTheme creates a huh theme
func NewAppTheme() *huh.Theme {
	return huh.ThemeBase()
}

// =============================================================================
// Utility Functions
// =============================================================================

// StringWidth calculates the visible width of a string
func StringWidth(s string) int {
	// Strip ANSI codes before measuring
	cleaned := stripANSI(s)
	return runewidth.StringWidth(cleaned)
}

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// =============================================================================
// Compatibility Layer (functions that other files expect)
// =============================================================================

// These variables provide compatibility with code that uses Style.Render() pattern
// They are simple wrappers that call the appropriate render function

// TitleStyle renders title text
var TitleStyle = styleRenderer{render: RenderTitle}

// NormalStyle renders normal text
var NormalStyle = styleRenderer{render: RenderNormal}

// SelectedStyle renders selected text
var SelectedStyle = styleRendererWithWidth{render: RenderSelectedWidth, renderSimple: RenderSelected}

// HintStyle renders hint text
var HintStyle = styleRenderer{render: RenderHint}

// AccentStyle renders accent text
var AccentStyle = styleRenderer{render: RenderAccent}

// ProgressStyle renders progress text
var ProgressStyle = styleRenderer{render: RenderProgress}

// DimStyle renders dim text
var DimStyle = styleRenderer{render: RenderDim}

// StatsStyle renders stats text
var StatsStyle = styleRenderer{render: RenderStats}

// DividerStyle renders divider text
var DividerStyle = styleRenderer{render: RenderDivider}

// DividerSelectedStyle renders selected divider text
var DividerSelectedStyle = styleRenderer{render: RenderDividerSelected}

// StatusMsgStyle renders status messages
var StatusMsgStyle = styleRenderer{render: RenderStatusMsg}

// TabActiveStyle renders active tabs
var TabActiveStyle = styleRenderer{render: RenderTabActive}

// TabInactiveStyle renders inactive tabs
var TabInactiveStyle = styleRenderer{render: RenderTabInactive}

// ArrowStyle renders arrows
var ArrowStyle = styleRenderer{render: RenderArrow}

// DescStyle renders descriptions (dim)
var DescStyle = styleRenderer{render: RenderDim}

// BorderStyle is a special style for borders
var BorderStyle = borderStyleRenderer{}

// BorderStyleWhite is a special style for white borders
var BorderStyleWhite = borderStyleRenderer{}

// borderStyleRendererWithColor provides border rendering with custom color
type borderStyleRendererWithColor struct {
	width       int
	height      int
	marginTop   int
	borderColor int
}

// Width sets the width for colored border
func (b borderStyleRendererWithColor) Width(w int) borderStyleRendererWithColor {
	return borderStyleRendererWithColor{width: w, height: b.height, marginTop: b.marginTop, borderColor: b.borderColor}
}

// Height sets the height for colored border
func (b borderStyleRendererWithColor) Height(h int) borderStyleRendererWithColor {
	return borderStyleRendererWithColor{width: b.width, height: h, marginTop: b.marginTop, borderColor: b.borderColor}
}

// MarginTop sets the margin top for colored border
func (b borderStyleRendererWithColor) MarginTop(n int) borderStyleRendererWithColor {
	return borderStyleRendererWithColor{width: b.width, height: b.height, marginTop: n, borderColor: b.borderColor}
}

// Render renders content with colored border
func (b borderStyleRendererWithColor) Render(content string) string {
	w := b.width
	h := b.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 20
	}
	// Add margin top (newlines before border)
	prefix := ""
	if b.marginTop > 0 {
		prefix = strings.Repeat("\n", b.marginTop)
	}
	return prefix + renderBorderWithColor(content, w, h, b.borderColor)
}

// NewBorderStyleWithColor creates a new border style with custom color
func NewBorderStyleWithColor(borderColor int) borderStyleRendererWithColor {
	return borderStyleRendererWithColor{
		width:       0,
		height:      0,
		marginTop:   0,
		borderColor: borderColor,
	}
}

// Report styles
var ReportTitleStyle = styleRenderer{render: func(s string) string { return style(s, colorPink, true) }}
var ReportSubtitleStyle = styleRenderer{render: func(s string) string { return style(s, colorCyan, false) }}
var ReportHeaderStyle = styleRenderer{render: func(s string) string { return style(s, colorPink, true) }}
var ReportCellStyle = styleRenderer{render: func(s string) string { return " " + s + " " }}
var ReportRowStyle = styleRenderer{render: func(s string) string { return style(" "+s+" ", colorWhite2, false) }}
var ReportStatStyle = styleRenderer{render: func(s string) string { return style(s, colorGreen, true) }}
var ReportBorderStyle = styleRenderer{render: func(s string) string { return style(s, colorPurple, false) }}
var ReportHighlightStyle = styleRenderer{render: func(s string) string { return style(" "+s+" ", colorYellow, true) }}
var ReportSuccessStyle = styleRenderer{render: func(s string) string { return style(s, colorGreen, true) }}
var ReportErrorStyle = styleRenderer{render: func(s string) string { return style(s, colorRed, true) }}
var ReportProgressStyle = styleRenderer{render: func(s string) string { return style(s, colorYellow, false) }}
var ReportSummaryStyle = styleRenderer{render: func(s string) string { return fg(colorCyan) + ansiItalic + s + ansiReset }}

// Row status styles
var PendingRowStyle = styleRenderer{render: func(s string) string { return style(s, colorPending, false) }}
var LinkedRowStyle = styleRenderer{render: func(s string) string { return style(s, colorLinked, false) }}
var DomainRowStyle = styleRenderer{render: func(s string) string { return style(s, colorDomain, false) }}

// ProgressFilledStyle for progress bars
var ProgressFilledStyle = styleRenderer{render: func(s string) string { return style(s, colorRed, false) }}
var ProgressEmptyStyle = styleRenderer{render: func(s string) string { return style(s, colorGray, false) }}

// =============================================================================
// Style Renderer Types (compatibility layer)
// =============================================================================

// styleRenderer provides a Render method for compatibility
type styleRenderer struct {
	render   func(string) string
	width    int
	boldFlag bool
	//renderFunc func(string) string // The base render function
}

func (s styleRenderer) Render(text string) string {
	result := text
	// Apply width padding if set
	if s.width > 0 {
		result = padRight(result, s.width)
	}
	// Apply the style rendering
	return s.render(result)
}

// Width returns a new styleRenderer with width set
func (s styleRenderer) Width(w int) styleRenderer {
	return styleRenderer{
		render:   s.render,
		width:    w,
		boldFlag: s.boldFlag,
	}
}

// Bold returns a new styleRenderer with bold attribute
func (s styleRenderer) Bold(b bool) styleRenderer {
	if b {
		// Create a new render function that adds bold
		baseRender := s.render
		return styleRenderer{
			render: func(text string) string {
				// The baseRender already adds color, we need to insert bold
				rendered := baseRender(text)
				// Insert bold after the first escape sequence
				return ansiBold + rendered
			},
			width:    s.width,
			boldFlag: true,
		}
	}
	return s
}

// styleRendererWithWidth provides Render and Width methods
type styleRendererWithWidth struct {
	render       func(string, int) string
	renderSimple func(string) string
	width        int
}

func (s styleRendererWithWidth) Render(text string) string {
	if s.width > 0 {
		return s.render(text, s.width)
	}
	return s.renderSimple(text)
}

func (s styleRendererWithWidth) Width(w int) styleRendererWithWidth {
	return styleRendererWithWidth{
		render:       s.render,
		renderSimple: s.renderSimple,
		width:        w,
	}
}

// borderStyleRenderer provides border rendering
type borderStyleRenderer struct {
	width     int
	height    int
	marginTop int
}

func (b borderStyleRenderer) Width(w int) borderStyleRenderer {
	return borderStyleRenderer{width: w, height: b.height, marginTop: b.marginTop}
}

func (b borderStyleRenderer) Height(h int) borderStyleRenderer {
	return borderStyleRenderer{width: b.width, height: h, marginTop: b.marginTop}
}

func (b borderStyleRenderer) MarginTop(n int) borderStyleRenderer {
	return borderStyleRenderer{width: b.width, height: b.height, marginTop: n}
}

func (b borderStyleRenderer) Render(content string) string {
	w := b.width
	h := b.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 20
	}
	// Add margin top (newlines before border)
	prefix := ""
	if b.marginTop > 0 {
		prefix = strings.Repeat("\n", b.marginTop)
	}
	return prefix + RenderBorder(content, w, h)
}

// =============================================================================
// Two-Box Layout Helpers (used by layerslayer.go selectors)
// =============================================================================

// RenderCenteredFooter renders a centered help text in a white-bordered footer box.
// This consolidates the repeated footer rendering pattern across all selector models.
func RenderCenteredFooter(helpText string, innerWidth int) string {
	textWidth := len(helpText)
	padding := (innerWidth - textWidth) / 2
	var footerContent strings.Builder
	if padding > 0 {
		footerContent.WriteString(strings.Repeat(" ", padding))
	}
	footerContent.WriteString(RenderHint(helpText))
	remaining := innerWidth - padding - textWidth
	if remaining > 0 {
		footerContent.WriteString(strings.Repeat(" ", remaining))
	}
	return RenderBorderWhite(footerContent.String(), innerWidth, 1)
}

// PadContentToHeight pads content with newlines to fill available height.
// Returns the padded content string.
func PadContentToHeight(content string, targetHeight int) string {
	contentLines := strings.Count(content, "\n")
	if contentLines < targetHeight {
		return content + strings.Repeat("\n", targetHeight-contentLines)
	}
	return content
}

// BuildTwoBoxView constructs the standard two-box layout used by most selector models.
// Returns the complete view string with main content box (red border) and footer box (white border).
func BuildTwoBoxView(content, helpText string, layout Layout) string {
	// Calculate available height for main content box
	// Subtract: footer box (3 lines: 1 content + 2 border) + spacing (1 line) + border overhead (2 lines)
	mainAvailableHeight := layout.ViewportHeight - 6
	if mainAvailableHeight < 10 {
		mainAvailableHeight = 10
	}

	// Pad content to fill available height
	paddedContent := PadContentToHeight(content, mainAvailableHeight)

	var result strings.Builder

	// First box: Main content (red border)
	mainBordered := RenderBorder(paddedContent, layout.InnerWidth, mainAvailableHeight)
	result.WriteString(mainBordered)
	result.WriteString("\n") // Spacing between boxes

	// Second box: Help text (white border, 1 row high)
	result.WriteString(RenderCenteredFooter(helpText, layout.InnerWidth))

	return result.String()
}

// =============================================================================
// Deprecated/Removed Functions
// =============================================================================

// BorderedBox is deprecated - use BorderStyle.Width().Height().Render() instead
func BorderedBox(layout Layout) borderStyleRenderer {
	return borderStyleRenderer{width: layout.InnerWidth, height: layout.ViewportHeight - 4}
}

// BorderedBoxDefault is deprecated
func BorderedBoxDefault() borderStyleRenderer {
	return BorderedBox(DefaultLayout())
}

// RenderRowWithColor is deprecated - use style() directly
func RenderRowWithColor(row string, _ interface{}) string {
	return row // Just return as-is, caller should use specific render functions
}
