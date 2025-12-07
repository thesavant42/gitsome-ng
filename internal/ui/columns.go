package ui

// columns.go provides generic column width calculation for bubbles/table.
// Use ColumnSpec and CalculateColumns() instead of duplicating percentage-based math.

import (
	"github.com/charmbracelet/bubbles/table"
)

// =============================================================================
// Column Specification Types
// =============================================================================

// ColumnSpec defines a table column with flexible or fixed width.
// Use FlexRatio for columns that should expand/contract with terminal width.
// Use FixedWidth for columns that should maintain constant width.
type ColumnSpec struct {
	Title      string
	MinWidth   int // Minimum width (0 = no minimum)
	FixedWidth int // If > 0, use this exact width (ignores FlexRatio)
	FlexRatio  int // Relative ratio for flexible columns (0 = fixed-only)
}

// =============================================================================
// Column Calculation
// =============================================================================

// CalculateColumns computes column widths from specs.
// Flexible columns split remaining space by ratio after fixed columns are allocated.
//
// Example:
//
//	columns := CalculateColumns([]ColumnSpec{
//	    {Title: "Name", FlexRatio: 30, MinWidth: 20},
//	    {Title: "Email", FlexRatio: 40, MinWidth: 25},
//	    {Title: "Commits", FixedWidth: 10},
//	}, layout.TableWidth)
//
// This allocates 10 chars to "Commits", then splits remaining space
// 30:40 between "Name" and "Email", respecting minimums.
func CalculateColumns(specs []ColumnSpec, totalWidth int) []table.Column {
	if totalWidth < 50 {
		totalWidth = 50
	}

	// First pass: allocate fixed widths and sum flex ratios
	fixedTotal := 0
	flexTotal := 0
	for _, s := range specs {
		if s.FixedWidth > 0 {
			fixedTotal += s.FixedWidth
		} else {
			flexTotal += s.FlexRatio
		}
	}

	remaining := totalWidth - fixedTotal
	if remaining < 0 {
		remaining = 0
	}

	// Second pass: calculate final widths
	columns := make([]table.Column, len(specs))
	for i, s := range specs {
		var width int
		if s.FixedWidth > 0 {
			width = s.FixedWidth
		} else if flexTotal > 0 {
			width = remaining * s.FlexRatio / flexTotal
		}

		// Apply minimum width constraint
		if s.MinWidth > 0 && width < s.MinWidth {
			width = s.MinWidth
		}

		columns[i] = table.Column{Title: s.Title, Width: width}
	}

	return columns
}

// =============================================================================
// Pre-defined Column Layouts
// =============================================================================

// CommitterColumns returns column specs for the main committer table.
// Matches the existing ColumnWidths system but uses the new ColumnSpec pattern.
func CommitterColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Tag", FixedWidth: ColWidthTag},
		{Title: "Rank", FixedWidth: ColWidthRank},
		{Title: "Name", FlexRatio: 20, MinWidth: ColWidthName},
		{Title: "GitHub Login", FlexRatio: 20, MinWidth: ColWidthLogin},
		{Title: "Email", FlexRatio: 40, MinWidth: ColWidthEmail},
		{Title: "Commits", FixedWidth: ColWidthCommits},
		{Title: "%", FixedWidth: ColWidthPercent},
	}
}

// LayerColumns returns column specs for Docker layer tables.
func LayerColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Index", FixedWidth: 8},
		{Title: "Size", FixedWidth: 12},
		{Title: "Digest", FlexRatio: 100, MinWidth: 20},
	}
}

// FileListColumns returns column specs for filesystem browser tables.
func FileListColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Mode", FixedWidth: 12},
		{Title: "Size", FixedWidth: 10},
		{Title: "Name", FlexRatio: 100, MinWidth: 30},
	}
}

// SingleColumnSpec returns a column spec for single-column selectors.
func SingleColumnSpec(title string) []ColumnSpec {
	return []ColumnSpec{
		{Title: title, FlexRatio: 100},
	}
}

// =============================================================================
// Column Width Helpers
// =============================================================================

// DistributeWidth distributes available width across columns by ratio.
// Returns slice of widths. Useful for custom column layouts.
//
// Example:
//
//	widths := DistributeWidth(100, []int{30, 40, 30})
//	// Returns [30, 40, 30]
func DistributeWidth(totalWidth int, ratios []int) []int {
	if len(ratios) == 0 {
		return nil
	}

	totalRatio := 0
	for _, r := range ratios {
		totalRatio += r
	}

	if totalRatio == 0 {
		// Equal distribution
		equalWidth := totalWidth / len(ratios)
		widths := make([]int, len(ratios))
		for i := range widths {
			widths[i] = equalWidth
		}
		return widths
	}

	widths := make([]int, len(ratios))
	for i, r := range ratios {
		widths[i] = totalWidth * r / totalRatio
	}
	return widths
}

// ClampWidth ensures width is within min/max bounds.
func ClampWidth(width, minWidth, maxWidth int) int {
	if minWidth > 0 && width < minWidth {
		return minWidth
	}
	if maxWidth > 0 && width > maxWidth {
		return maxWidth
	}
	return width
}
