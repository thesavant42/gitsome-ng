package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thesavant42/gitsome-ng/internal/models"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection and initializes the schema
func New(dbPath string) (*DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if _, err := conn.Exec(createCommitsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create commits schema: %w", err)
	}

	// Initialize links table
	if _, err := conn.Exec(createLinksTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create links schema: %w", err)
	}

	// Initialize tags table
	if _, err := conn.Exec(createTagsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create tags schema: %w", err)
	}

	// Initialize highlight domains table
	if _, err := conn.Exec(createDomainsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create domains schema: %w", err)
	}

	// Initialize tracked repos table
	if _, err := conn.Exec(createTrackedReposTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create tracked repos schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// ListProjectFiles returns a list of .db files in the given directory
func ListProjectFiles(dir string) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	
	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".db" {
			projects = append(projects, name)
		}
	}
	return projects, nil
}

// InsertCommits inserts multiple commits into the database
func (db *DB) InsertCommits(records []models.CommitRecord) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertCommit)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.Exec(
			r.SHA,
			r.Message,
			r.AuthorName,
			r.AuthorEmail,
			r.AuthorDate.Format("2006-01-02T15:04:05Z"),
			r.CommitterName,
			r.CommitterEmail,
			r.CommitterDate.Format("2006-01-02T15:04:05Z"),
			r.GitHubAuthorLogin,
			r.GitHubCommitterLogin,
			r.HTMLURL,
			r.RepoOwner,
			r.RepoName,
		)
		if err != nil {
			return fmt.Errorf("failed to insert commit %s: %w", r.SHA, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetCommitterStats returns contributor statistics grouped by committer
func (db *DB) GetCommitterStats(repoOwner, repoName string) ([]models.ContributorStats, int, error) {
	total, err := db.getTotalCommits(repoOwner, repoName)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.conn.Query(selectCommitterStats, repoOwner, repoName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query committer stats: %w", err)
	}
	defer rows.Close()

	var stats []models.ContributorStats
	for rows.Next() {
		var s models.ContributorStats
		if err := rows.Scan(&s.Name, &s.Email, &s.GitHubLogin, &s.CommitCount); err != nil {
			return nil, 0, fmt.Errorf("failed to scan row: %w", err)
		}
		if total > 0 {
			s.Percentage = float64(s.CommitCount) / float64(total) * 100
		}
		stats = append(stats, s)
	}

	return stats, total, nil
}

// GetAuthorStats returns contributor statistics grouped by author
func (db *DB) GetAuthorStats(repoOwner, repoName string) ([]models.ContributorStats, int, error) {
	total, err := db.getTotalCommits(repoOwner, repoName)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.conn.Query(selectAuthorStats, repoOwner, repoName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query author stats: %w", err)
	}
	defer rows.Close()

	var stats []models.ContributorStats
	for rows.Next() {
		var s models.ContributorStats
		if err := rows.Scan(&s.Name, &s.Email, &s.GitHubLogin, &s.CommitCount); err != nil {
			return nil, 0, fmt.Errorf("failed to scan row: %w", err)
		}
		if total > 0 {
			s.Percentage = float64(s.CommitCount) / float64(total) * 100
		}
		stats = append(stats, s)
	}

	return stats, total, nil
}

func (db *DB) getTotalCommits(repoOwner, repoName string) (int, error) {
	var total int
	err := db.conn.QueryRow(selectTotalCommits, repoOwner, repoName).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total commits: %w", err)
	}
	return total, nil
}

// GetLatestCommitSHA returns the most recent commit SHA for a repository
func (db *DB) GetLatestCommitSHA(repoOwner, repoName string) (string, error) {
	var sha string
	err := db.conn.QueryRow(`
		SELECT sha FROM commits 
		WHERE repo_owner = ? AND repo_name = ? 
		ORDER BY committer_date DESC 
		LIMIT 1
	`, repoOwner, repoName).Scan(&sha)
	if err == sql.ErrNoRows {
		return "", nil // No commits yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get latest commit SHA: %w", err)
	}
	return sha, nil
}

// GetNextGroupID returns the next available group ID for linking
func (db *DB) GetNextGroupID(repoOwner, repoName string) (int, error) {
	var maxID int
	err := db.conn.QueryRow(selectMaxGroupID, repoOwner, repoName).Scan(&maxID)
	if err != nil {
		return 0, fmt.Errorf("failed to get max group ID: %w", err)
	}
	return maxID + 1, nil
}

// SaveLink saves a link between a committer email and a group
func (db *DB) SaveLink(repoOwner, repoName string, groupID int, email string) error {
	_, err := db.conn.Exec(insertLink, groupID, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to save link: %w", err)
	}
	return nil
}

// GetLinks returns a map of committer email to group ID for a repository
func (db *DB) GetLinks(repoOwner, repoName string) (map[string]int, error) {
	rows, err := db.conn.Query(selectLinks, repoOwner, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to query links: %w", err)
	}
	defer rows.Close()

	links := make(map[string]int)
	for rows.Next() {
		var email string
		var groupID int
		if err := rows.Scan(&email, &groupID); err != nil {
			return nil, fmt.Errorf("failed to scan link: %w", err)
		}
		links[email] = groupID
	}
	return links, nil
}

// RemoveLink removes a link for a committer
func (db *DB) RemoveLink(repoOwner, repoName, email string) error {
	_, err := db.conn.Exec(deleteLink, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to remove link: %w", err)
	}
	return nil
}

// SaveTag tags a committer for future bulk actions
func (db *DB) SaveTag(repoOwner, repoName, email string) error {
	_, err := db.conn.Exec(insertTag, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to save tag: %w", err)
	}
	return nil
}

// RemoveTag removes a tag from a committer
func (db *DB) RemoveTag(repoOwner, repoName, email string) error {
	_, err := db.conn.Exec(deleteTag, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to remove tag: %w", err)
	}
	return nil
}

// GetTags returns a set of tagged committer emails for a repository
func (db *DB) GetTags(repoOwner, repoName string) (map[string]bool, error) {
	rows, err := db.conn.Query(selectTags, repoOwner, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close()

	tags := make(map[string]bool)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags[email] = true
	}
	return tags, nil
}

// HasCachedCommits checks if there are any cached commits for a repository
func (db *DB) HasCachedCommits(repoOwner, repoName string) (bool, error) {
	var count int
	err := db.conn.QueryRow(selectTotalCommits, repoOwner, repoName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check cached commits: %w", err)
	}
	return count > 0, nil
}

// SaveDomain saves a highlight domain with its color index (global - shared across all repos)
func (db *DB) SaveDomain(domain string, colorIndex int) error {
	_, err := db.conn.Exec(insertDomain, domain, colorIndex)
	if err != nil {
		return fmt.Errorf("failed to save domain: %w", err)
	}
	return nil
}

// GetDomains returns a map of domain to color index (global - shared across all repos)
func (db *DB) GetDomains() (map[string]int, error) {
	rows, err := db.conn.Query(selectDomains)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	domains := make(map[string]int)
	for rows.Next() {
		var domain string
		var colorIndex int
		if err := rows.Scan(&domain, &colorIndex); err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains[domain] = colorIndex
	}
	return domains, nil
}

// GetNextDomainColorIndex returns the next available color index for domains (global)
func (db *DB) GetNextDomainColorIndex() (int, error) {
	var maxIndex int
	err := db.conn.QueryRow(selectMaxDomainColorIndex).Scan(&maxIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get max domain color index: %w", err)
	}
	return maxIndex + 1, nil
}

// RemoveDomain removes a highlight domain (global)
func (db *DB) RemoveDomain(domain string) error {
	_, err := db.conn.Exec(deleteDomain, domain)
	if err != nil {
		return fmt.Errorf("failed to remove domain: %w", err)
	}
	return nil
}

// AddTrackedRepo adds a repository to the tracked repos list
func (db *DB) AddTrackedRepo(repoOwner, repoName string) error {
	_, err := db.conn.Exec(insertTrackedRepo, repoOwner, repoName)
	if err != nil {
		return fmt.Errorf("failed to add tracked repo: %w", err)
	}
	return nil
}

// GetTrackedRepos returns all tracked repositories
func (db *DB) GetTrackedRepos() ([]models.RepoInfo, error) {
	rows, err := db.conn.Query(selectTrackedRepos)
	if err != nil {
		return nil, fmt.Errorf("failed to query tracked repos: %w", err)
	}
	defer rows.Close()

	var repos []models.RepoInfo
	for rows.Next() {
		var r models.RepoInfo
		var addedAt string
		if err := rows.Scan(&r.Owner, &r.Name, &addedAt); err != nil {
			return nil, fmt.Errorf("failed to scan tracked repo: %w", err)
		}
		// Parse the timestamp
		r.AddedAt, _ = parseTimestamp(addedAt)
		repos = append(repos, r)
	}
	return repos, nil
}

// RemoveTrackedRepo removes a repository from tracking
func (db *DB) RemoveTrackedRepo(repoOwner, repoName string) error {
	_, err := db.conn.Exec(deleteTrackedRepo, repoOwner, repoName)
	if err != nil {
		return fmt.Errorf("failed to remove tracked repo: %w", err)
	}
	return nil
}

// GetCombinedCommitterStats returns committer stats across all repos, deduplicated by GitHub login
func (db *DB) GetCombinedCommitterStats() ([]models.ContributorStats, int, error) {
	// Get total commits across all repos
	var total int
	err := db.conn.QueryRow(selectCombinedTotalCommits).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get combined total commits: %w", err)
	}

	rows, err := db.conn.Query(selectCombinedCommitterStats)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query combined committer stats: %w", err)
	}
	defer rows.Close()

	var stats []models.ContributorStats
	for rows.Next() {
		var s models.ContributorStats
		if err := rows.Scan(&s.Name, &s.Email, &s.GitHubLogin, &s.CommitCount); err != nil {
			return nil, 0, fmt.Errorf("failed to scan combined stats row: %w", err)
		}
		if total > 0 {
			s.Percentage = float64(s.CommitCount) / float64(total) * 100
		}
		stats = append(stats, s)
	}

	return stats, total, nil
}

// parseTimestamp parses SQLite timestamp formats
func parseTimestamp(ts string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	}
	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", ts)
}

