package ui

import (
	"fmt"
	"strings"

	"github.com/thesavant42/gitsome-ng/internal/models"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Color palette
	purple = lipgloss.Color("99")  // for borders
	pink   = lipgloss.Color("205") // for header text
	cyan   = lipgloss.Color("86")
	white  = lipgloss.Color("255")
	green  = lipgloss.Color("82")
	yellow = lipgloss.Color("220")

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(pink).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(cyan).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Foreground(pink).
			Bold(true).
			Align(lipgloss.Center)

	cellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	oddRowStyle = cellStyle.Foreground(white)

	evenRowStyle = cellStyle.Foreground(white)

	statStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	borderStyle = lipgloss.NewStyle().
			Foreground(purple)

	highlightStyle = cellStyle.
			Foreground(yellow).
			Bold(true)
)

// PrintHeader prints a styled header for the report
func PrintHeader(owner, repo string, totalCommits int) {
	header := titleStyle.Render(fmt.Sprintf("Commit Statistics for %s/%s", owner, repo))
	stats := subtitleStyle.Render(fmt.Sprintf("Total Commits: %s", statStyle.Render(fmt.Sprintf("%d", totalCommits))))

	fmt.Println()
	fmt.Println(header)
	fmt.Println(stats)
	fmt.Println()
}

// PrintContributorTable prints a styled table of contributor statistics
//
// CORRECT LIPGLOSS USAGE: This is a CLI report (non-interactive), so we use manual
// formatting with strings. Lipgloss is used ONLY for colors/styling the output text.
// We DO NOT use lipgloss to build table structure - we use string formatting instead.
//
// For interactive TUI tables, use bubbles/table component (see internal/ui/tui.go).
// See docs/LIPGLOSS_FORBIDDEN_PATTERNS.md for forbidden patterns.
func PrintContributorTable(title string, stats []models.ContributorStats, highlight string) {
	if len(stats) == 0 {
		fmt.Println(subtitleStyle.Render(title + ": No data"))
		return
	}

	// Print section title
	fmt.Println(titleStyle.Render(title))

	// Track which rows should be highlighted
	highlightRows := make(map[int]bool)
	highlightLower := strings.ToLower(highlight)

	for i, s := range stats {
		// Check if this row should be highlighted (case-insensitive)
		if highlight != "" {
			rowText := strings.ToLower(s.Name + s.GitHubLogin + s.Email)
			if strings.Contains(rowText, highlightLower) {
				highlightRows[i] = true
			}
		}
	}

	// Calculate column widths
	colWidths := []int{6, 20, 15, 25, 10, 8} // Rank, Name, Login, Email, Commits, %
	totalWidth := 2                          // Start with left border
	for _, w := range colWidths {
		totalWidth += w + 3 // column width + " │ " separator
	}
	totalWidth -= 1 // Last column doesn't have trailing separator

	// Build border line
	separator := strings.Repeat("─", totalWidth-2) // -2 for corner chars

	// Print top border
	fmt.Println(borderStyle.Render("┌" + separator + "┐"))

	// Print header
	headerRow := fmt.Sprintf("│ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │",
		colWidths[0], "Rank",
		colWidths[1], "Name",
		colWidths[2], "GitHub Login",
		colWidths[3], "Email",
		colWidths[4], "Commits",
		colWidths[5], "%")
	fmt.Println(headerStyle.Render(headerRow))

	// Print separator line
	fmt.Println(borderStyle.Render("├" + separator + "┤"))

	// Print rows
	for i, s := range stats {
		rank := fmt.Sprintf("%d", i+1)
		name := s.Name
		login := s.GitHubLogin
		if login == "" {
			login = "-"
		}
		email := s.Email
		commits := fmt.Sprintf("%d", s.CommitCount)
		pct := fmt.Sprintf("%.1f%%", s.Percentage)

		// Truncate long values
		if len(name) > colWidths[1] {
			name = name[:colWidths[1]-3] + "..."
		}
		if len(login) > colWidths[2] {
			login = login[:colWidths[2]-3] + "..."
		}
		if len(email) > colWidths[3] {
			email = email[:colWidths[3]-3] + "..."
		}

		rowText := fmt.Sprintf("│ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │",
			colWidths[0], rank,
			colWidths[1], name,
			colWidths[2], login,
			colWidths[3], email,
			colWidths[4], commits,
			colWidths[5], pct)

		// Apply styling based on highlight/row type
		if highlightRows[i] {
			fmt.Println(highlightStyle.Render(rowText))
		} else if i%2 == 0 {
			fmt.Println(evenRowStyle.Render(rowText))
		} else {
			fmt.Println(oddRowStyle.Render(rowText))
		}
	}

	// Print bottom border
	fmt.Println(borderStyle.Render("└" + separator + "┘"))
	fmt.Println()
}

// PrintProgress prints a progress message during fetch
func PrintProgress(fetched, page int) {
	progressStyle := lipgloss.NewStyle().Foreground(yellow)
	fmt.Printf("\r%s", progressStyle.Render(fmt.Sprintf("Fetching commits... Page %d (%d commits)", page, fetched)))
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	successStyle := lipgloss.NewStyle().
		Foreground(green).
		Bold(true)
	fmt.Println(successStyle.Render(message))
}

// PrintError prints an error message
func PrintError(message string) {
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)
	fmt.Println(errorStyle.Render("Error: " + message))
}

// PrintSummary prints a brief summary after the tables
func PrintSummary(committerCount, authorCount, totalCommits int) {
	summaryStyle := lipgloss.NewStyle().
		Foreground(cyan).
		Italic(true)

	summary := fmt.Sprintf(
		"Summary: %d unique committers, %d unique authors across %d commits",
		committerCount, authorCount, totalCommits,
	)

	fmt.Println(summaryStyle.Render(summary))
	fmt.Println()
}

// GenerateMarkdownReport generates a markdown report of the commit statistics
func GenerateMarkdownReport(owner, repo string, totalCommits int, committerStats, authorStats []models.ContributorStats) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Commit Statistics for %s/%s\n\n", owner, repo))
	sb.WriteString(fmt.Sprintf("**Total Commits:** %d\n\n", totalCommits))

	// Committers table
	sb.WriteString("## Committers (ranked by commit count)\n\n")
	sb.WriteString(generateMarkdownTable(committerStats))
	sb.WriteString("\n")

	// Authors table
	sb.WriteString("## Authors (ranked by commit count)\n\n")
	sb.WriteString(generateMarkdownTable(authorStats))
	sb.WriteString("\n")

	// Summary
	sb.WriteString(fmt.Sprintf("**Summary:** %d unique committers, %d unique authors across %d commits\n",
		len(committerStats), len(authorStats), totalCommits))

	return sb.String()
}

// generateMarkdownTable generates a markdown table from contributor stats
func generateMarkdownTable(stats []models.ContributorStats) string {
	if len(stats) == 0 {
		return "No data\n"
	}

	var sb strings.Builder

	// Header
	sb.WriteString("| Rank | Name | GitHub Login | Email | Commits | % |\n")
	sb.WriteString("|------|------|--------------|-------|---------|---|\n")

	// Rows
	for i, s := range stats {
		login := s.GitHubLogin
		if login == "" {
			login = "-"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %d | %.1f%% |\n",
			i+1, s.Name, login, s.Email, s.CommitCount, s.Percentage))
	}

	return sb.String()
}
