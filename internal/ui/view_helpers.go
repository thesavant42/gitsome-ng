package ui

// view_helpers.go provides common View() rendering helpers.
// Use these to build consistent two-box layouts across all TUI models.

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
)

// =============================================================================
// Table Rendering with Full-Width Selection
// =============================================================================

// RenderTableWithSelection renders a bubbles table with full-width selection highlight.
// The table's Selected style should use Background(lipgloss.NoColor{}) to prevent ANSI embedding,
// and this function applies the visible selection styling.
//
// CRITICAL: Understanding bubbles/table View() output:
// - Line 0: Header row (with visual bottom border via lipgloss BorderBottom, but NOT a separate line)
// - Line 1+: Data rows (only visible rows due to viewport scrolling)
// - There is NO separate divider line from bubbles - we add one manually for consistency
//
// This handles scrolling correctly by calculating the visible cursor position based on
// the table's height and current cursor position.
//
// This is THE standard way to render tables - use it everywhere for consistent selection behavior.
func RenderTableWithSelection(t table.Model, layout Layout) string {
	tableOutput := t.View()
	lines := strings.Split(tableOutput, "\n")
	var result []string

	cursor := t.Cursor()

	// Calculate visible cursor index based on table scrolling
	// Table height is the number of visible data rows (doesn't include header)
	height := t.Height()
	totalRows := len(t.Rows())
	start := 0
	if cursor >= height {
		start = cursor - height + 1
	}
	if start > totalRows-height {
		start = totalRows - height
	}
	if start < 0 {
		start = 0
	}
	visibleCursorIndex := cursor - start

	for i, line := range lines {
		// Header row (line 0) - render with full width, then add our divider
		if i == 0 {
			result = append(result, NormalStyle.Width(layout.InnerWidth).Render(line))
			// Add divider after header - use InnerWidth for full width
			result = append(result, strings.Repeat("─", layout.InnerWidth))
			continue
		}

		// Data rows start at line 1 in the bubbles output (line 0 is header)
		// dataRowIndex is 0-based index into visible rows
		dataRowIndex := i - 1

		// Apply full-width selection styling to the visible cursor row
		// Strip ANSI codes first to prevent embedded reset codes from killing the background
		if dataRowIndex == visibleCursorIndex {
			cleanLine := stripANSI(line)
			result = append(result, SelectedStyle.Width(layout.InnerWidth).Render(cleanLine))
			continue
		}

		// Non-selected data rows - apply normal text color with full width
		result = append(result, NormalStyle.Width(layout.InnerWidth).Render(line))
	}

	return strings.Join(result, "\n")
}

// =============================================================================
// View Header - Title + Divider Pattern
// =============================================================================

// ViewHeader renders title + full-width divider + spacing.
// Use at the start of all View() content to ensure consistent headers.
//
// Example:
//
//	content := ViewHeader("Select Project", layout.InnerWidth)
//	content += m.renderProjectList()
//	return BuildTwoBoxView(content, "up/down: navigate", layout)
func ViewHeader(title string, innerWidth int) string {
	var b strings.Builder
	b.WriteString(RenderTitle(title))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", innerWidth))
	b.WriteString("\n\n")
	return b.String()
}

// ViewHeaderWithSubtitle renders title + subtitle + divider + spacing.
func ViewHeaderWithSubtitle(title, subtitle string, innerWidth int) string {
	var b strings.Builder
	b.WriteString(RenderTitle(title))
	b.WriteString("\n")
	if subtitle != "" {
		b.WriteString(RenderDim(subtitle))
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("─", innerWidth))
	b.WriteString("\n\n")
	return b.String()
}

// =============================================================================
// Text Centering
// =============================================================================

// CenterText centers text within given width.
// Uses StringWidth() for accurate ANSI-aware width calculation.
func CenterText(text string, width int) string {
	textW := StringWidth(text)
	if textW >= width {
		return text
	}
	padding := (width - textW) / 2
	return strings.Repeat(" ", padding) + text
}

// CenterTextPadded centers text and pads to full width.
func CenterTextPadded(text string, width int) string {
	textW := StringWidth(text)
	if textW >= width {
		return text
	}
	leftPad := (width - textW) / 2
	rightPad := width - textW - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

// =============================================================================
// Content Padding
// =============================================================================

// PadToHeight pads content with newlines to fill target height.
// Alias for PadContentToHeight in styles.go for discoverability.
func PadToHeight(content string, targetHeight int) string {
	return PadContentToHeight(content, targetHeight)
}

// =============================================================================
// Two-Box Layout (delegates to styles.go)
// =============================================================================

// TwoBoxView constructs the standard two-box layout.
// This is an alias for BuildTwoBoxView in styles.go for API consistency.
//
// Layout:
//
//	┌────────────────────────┐
//	│ Main content           │  <- Red border
//	│                        │
//	│                        │
//	└────────────────────────┘
//	┌────────────────────────┐
//	│   Centered help text   │  <- White border, 1 row
//	└────────────────────────┘
//
// Example:
//
//	func (m myModel) View() string {
//	    content := ViewHeader("Title", m.layout.InnerWidth)
//	    content += m.Table.View()
//	    return TwoBoxView(content, "up/down: nav | Enter: select", m.layout)
//	}
func TwoBoxView(content, helpText string, layout Layout) string {
	return BuildTwoBoxView(content, helpText, layout)
}

// =============================================================================
// Dividers and Separators
// =============================================================================

// FullWidthDivider returns a horizontal divider spanning the inner width.
func FullWidthDivider(innerWidth int) string {
	return strings.Repeat("─", innerWidth)
}

// SpacedDivider returns a divider with blank lines before and after.
func SpacedDivider(innerWidth int) string {
	return "\n" + strings.Repeat("─", innerWidth) + "\n\n"
}

// =============================================================================
// List Item Rendering
// =============================================================================

// RenderListItem renders a list item with bullet and optional selection highlight.
func RenderListItem(text string, selected bool, width int) string {
	prefix := "• "
	if selected {
		return RenderSelectedWidth(prefix+text, width)
	}
	return RenderNormal(prefix + text)
}

// RenderNumberedItem renders a numbered list item with optional selection highlight.
func RenderNumberedItem(number int, text string, selected bool, width int) string {
	prefix := strings.Repeat(" ", 2-len(string(rune('0'+number%10)))) // Right-align single digits
	prefix += string(rune('0'+number%10)) + ". "
	if selected {
		return RenderSelectedWidth(prefix+text, width)
	}
	return RenderNormal(prefix + text)
}
