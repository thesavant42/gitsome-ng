package ui

import (
	"fmt"
	"strings"

	"charming-commits/internal/models"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

var (
	// Color palette
	purple    = lipgloss.Color("99")  // for borders
	pink      = lipgloss.Color("205") // for header text
	cyan      = lipgloss.Color("86")
	white     = lipgloss.Color("255")
	green     = lipgloss.Color("82")
	yellow    = lipgloss.Color("220")

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
func PrintContributorTable(title string, stats []models.ContributorStats, highlight string) {
	if len(stats) == 0 {
		fmt.Println(subtitleStyle.Render(title + ": No data"))
		return
	}

	// Print section title
	fmt.Println(titleStyle.Render(title))

	// Build table rows and track which rows should be highlighted
	rows := make([][]string, len(stats))
	highlightRows := make(map[int]bool)
	highlightLower := strings.ToLower(highlight)

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

		rows[i] = []string{rank, name, login, email, commits, pct}

		// Check if this row should be highlighted (case-insensitive)
		if highlight != "" {
			rowText := strings.ToLower(name + login + email)
			if strings.Contains(rowText, highlightLower) {
				highlightRows[i] = true
			}
		}
	}

	// Create styled table
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle
			case highlightRows[row]:
				return highlightStyle
			case row%2 == 0:
				return evenRowStyle
			default:
				return oddRowStyle
			}
		}).
		Headers("Rank", "Name", "GitHub Login", "Email", "Commits", "%").
		Rows(rows...)

	fmt.Println(t)
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

