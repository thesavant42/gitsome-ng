package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charming-commits/internal/models"
)

// ExportTabToMarkdown exports the current stats to a markdown file
func ExportTabToMarkdown(stats []models.ContributorStats, repoOwner, repoName string, totalCommits int, showCombined bool) (string, error) {
	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02")
	var filename string
	if showCombined {
		filename = fmt.Sprintf("combined-%s.md", timestamp)
	} else {
		// Sanitize repo name for filename
		safeOwner := strings.ReplaceAll(repoOwner, "/", "-")
		safeName := strings.ReplaceAll(repoName, "/", "-")
		filename = fmt.Sprintf("%s-%s-%s.md", safeOwner, safeName, timestamp)
	}

	// Build markdown content
	var sb strings.Builder

	// Title
	if showCombined {
		sb.WriteString("# Combined Committer Statistics\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("# Committer Statistics for %s/%s\n\n", repoOwner, repoName))
	}

	// Summary
	sb.WriteString(fmt.Sprintf("**Total Committers:** %d\n", len(stats)))
	sb.WriteString(fmt.Sprintf("**Total Commits:** %d\n", totalCommits))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Table header
	sb.WriteString("| Rank | Name | GitHub Login | Email | Commits | % |\n")
	sb.WriteString("|------|------|--------------|-------|---------|---|\n")

	// Table rows
	for i, s := range stats {
		login := s.GitHubLogin
		if login == "" {
			login = "-"
		} else {
			// Make GitHub login a clickable link
			login = fmt.Sprintf("[%s](https://github.com/%s)", login, login)
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %d | %.1f%% |\n",
			i+1, s.Name, login, s.Email, s.CommitCount, s.Percentage))
	}

	// Write to file
	err := os.WriteFile(filename, []byte(sb.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write markdown file: %w", err)
	}

	return filename, nil
}

// ExportDatabaseBackup copies the current database to a backup file
func ExportDatabaseBackup(currentDBPath string) (string, error) {
	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	baseName := strings.TrimSuffix(filepath.Base(currentDBPath), filepath.Ext(currentDBPath))
	backupFilename := fmt.Sprintf("%s-backup-%s.db", baseName, timestamp)

	// Open source file
	src, err := os.Open(currentDBPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(backupFilename)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	// Copy contents
	_, err = io.Copy(dst, src)
	if err != nil {
		return "", fmt.Errorf("failed to copy database: %w", err)
	}

	return backupFilename, nil
}

