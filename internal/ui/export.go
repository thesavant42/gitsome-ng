package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thesavant42/gitsome-ng/internal/db"
	"github.com/thesavant42/gitsome-ng/internal/models"
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

// ExportProjectReport exports a comprehensive project report to markdown
func ExportProjectReport(database *db.DB, dbPath string) (string, error) {
	if database == nil {
		return "", fmt.Errorf("no database connection")
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("project-report-%s.md", timestamp)

	// Get all tracked repos
	repos, err := database.GetTrackedRepos()
	if err != nil {
		return "", fmt.Errorf("failed to get tracked repos: %w", err)
	}

	// Get combined stats (all committers deduplicated)
	stats, totalCommits, err := database.GetCombinedCommitterStats()
	if err != nil {
		return "", fmt.Errorf("failed to get combined stats: %w", err)
	}

	var sb strings.Builder

	// Header
	dbName := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
	sb.WriteString(fmt.Sprintf("# Project Report: %s\n\n", dbName))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Tracked Repositories section
	sb.WriteString("## Tracked Repositories\n\n")
	if len(repos) == 0 {
		sb.WriteString("*No repositories tracked*\n\n")
	} else {
		for _, repo := range repos {
			repoURL := fmt.Sprintf("https://github.com/%s/%s", repo.Owner, repo.Name)
			// Get commit count for this repo
			_, repoCommits, _ := database.GetCommitterStats(repo.Owner, repo.Name)
			sb.WriteString(fmt.Sprintf("- [%s/%s](%s) - %d commits\n", repo.Owner, repo.Name, repoURL, repoCommits))
		}
		sb.WriteString("\n")
	}

	// Committers section
	sb.WriteString(fmt.Sprintf("## Committers (%d total, %d commits)\n\n", len(stats), totalCommits))

	// Group stats by GitHub login to consolidate emails
	type committerInfo struct {
		name       string
		login      string
		emails     []string
		commits    int
		percentage float64
	}

	// Build a map of login -> committerInfo, handling users with and without logins
	committers := make(map[string]*committerInfo)
	var order []string // preserve order

	for _, s := range stats {
		key := s.GitHubLogin
		if key == "" {
			key = s.Email // use email as key if no login
		}

		if existing, ok := committers[key]; ok {
			// Add email if not already present
			found := false
			for _, e := range existing.emails {
				if e == s.Email {
					found = true
					break
				}
			}
			if !found {
				existing.emails = append(existing.emails, s.Email)
			}
			existing.commits += s.CommitCount
			existing.percentage += s.Percentage
		} else {
			committers[key] = &committerInfo{
				name:       s.Name,
				login:      s.GitHubLogin,
				emails:     []string{s.Email},
				commits:    s.CommitCount,
				percentage: s.Percentage,
			}
			order = append(order, key)
		}
	}

	// Output each committer
	for _, key := range order {
		c := committers[key]

		// Committer header with GitHub link if available
		if c.login != "" {
			sb.WriteString(fmt.Sprintf("### [%s](https://github.com/%s)\n\n", c.login, c.login))
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n\n", c.name))
		}

		// Emails
		sb.WriteString(fmt.Sprintf("**Emails:** %s\n\n", strings.Join(c.emails, ", ")))
		sb.WriteString(fmt.Sprintf("**Commits:** %d (%.1f%%)\n\n", c.commits, c.percentage))

		// If user has a GitHub login, try to get their repos and gists
		if c.login != "" {
			hasData, _ := database.UserHasData(c.login)
			if hasData {
				// Get user repositories
				userRepos, err := database.GetUserRepositories(c.login)
				if err == nil && len(userRepos) > 0 {
					sb.WriteString(fmt.Sprintf("#### Repositories (%d)\n\n", len(userRepos)))
					sb.WriteString("| Name | Stars | Forks | Visibility |\n")
					sb.WriteString("|------|-------|-------|------------|\n")
					for _, repo := range userRepos {
						repoName := repo.Name
						if repo.URL != "" {
							repoName = fmt.Sprintf("[%s](%s)", repo.Name, repo.URL)
						}
						sb.WriteString(fmt.Sprintf("| %s | %d | %d | %s |\n",
							repoName, repo.StargazerCount, repo.ForkCount, repo.Visibility))
					}
					sb.WriteString("\n")
				}

				// Get user gists with files
				userGists, err := database.GetUserGists(c.login)
				if err == nil && len(userGists) > 0 {
					// Count total files
					totalFiles := 0
					for _, gist := range userGists {
						files, _ := database.GetGistFiles(gist.ID)
						totalFiles += len(files)
					}

					if totalFiles > 0 {
						sb.WriteString(fmt.Sprintf("#### Gist Files (%d)\n\n", totalFiles))
						sb.WriteString("| File | Language | Size | Gist |\n")
						sb.WriteString("|------|----------|------|------|\n")
						for _, gist := range userGists {
							files, _ := database.GetGistFiles(gist.ID)
							for _, file := range files {
								fileName := file.Name
								if gist.URL != "" {
									fileName = fmt.Sprintf("[%s](%s)", file.Name, gist.URL)
								}
								lang := file.Language
								if lang == "" {
									lang = "-"
								}
								// Format size
								sizeStr := fmt.Sprintf("%dB", file.Size)
								if file.Size >= 1024 {
									sizeStr = fmt.Sprintf("%.1fKB", float64(file.Size)/1024)
								}
								// Gist description or ID
								gistDesc := gist.Description
								if gistDesc == "" {
									gistDesc = gist.Name
								}
								if len(gistDesc) > 20 {
									gistDesc = gistDesc[:17] + "..."
								}
								sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
									fileName, lang, sizeStr, gistDesc))
							}
						}
						sb.WriteString("\n")
					}
				}
			} else {
				sb.WriteString("*No scanned data available*\n\n")
			}
		} else {
			sb.WriteString("*No GitHub profile*\n\n")
		}

		sb.WriteString("---\n\n")
	}

	// Write to file
	err = os.WriteFile(filename, []byte(sb.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write report file: %w", err)
	}

	return filename, nil
}

