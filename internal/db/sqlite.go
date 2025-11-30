package db

import (
	"database/sql"
	"encoding/json"
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

	// Initialize user repositories table
	if _, err := conn.Exec(createUserRepositoriesTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create user repositories schema: %w", err)
	}

	// Initialize user gists table
	if _, err := conn.Exec(createUserGistsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create user gists schema: %w", err)
	}

	// Initialize gist files table
	if _, err := conn.Exec(createGistFilesTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create gist files schema: %w", err)
	}

	// Initialize gist comments table
	if _, err := conn.Exec(createGistCommentsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create gist comments schema: %w", err)
	}

	// Initialize user profiles table
	if _, err := conn.Exec(createUserProfilesTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create user profiles schema: %w", err)
	}

	// Initialize API logs table
	if _, err := conn.Exec(createAPILogsTable); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create API logs schema: %w", err)
	}

	// Run migrations to add new columns to existing tables
	// These will silently fail if columns already exist
	migrations := []string{
		"ALTER TABLE user_repositories ADD COLUMN commit_count INTEGER DEFAULT 0",
		"ALTER TABLE user_gists ADD COLUMN revision_count INTEGER DEFAULT 0",
		"ALTER TABLE user_repositories ADD COLUMN primary_language TEXT",
		"ALTER TABLE user_repositories ADD COLUMN license_name TEXT",
		"ALTER TABLE user_gists ADD COLUMN fork_count INTEGER DEFAULT 0",
		"ALTER TABLE user_profiles ADD COLUMN organizations TEXT",
	}
	for _, migration := range migrations {
		conn.Exec(migration) // Ignore errors - column may already exist
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

// SaveUserRepository saves a user's repository to the database
func (db *DB) SaveUserRepository(repo models.UserRepository) error {
	_, err := db.conn.Exec(insertUserRepository,
		repo.GitHubLogin, repo.Name, repo.OwnerLogin, repo.Description,
		repo.URL, repo.SSHURL, repo.HomepageURL, repo.DiskUsage,
		repo.StargazerCount, repo.ForkCount, repo.CommitCount, repo.IsFork, repo.IsEmpty,
		repo.IsInOrganization, repo.HasWikiEnabled, repo.Visibility,
		repo.PrimaryLanguage, repo.LicenseName,
		repo.CreatedAt, repo.UpdatedAt, repo.PushedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save user repository: %w", err)
	}
	return nil
}

// SaveUserRepositories saves multiple user repositories in a transaction
func (db *DB) SaveUserRepositories(repos []models.UserRepository) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertUserRepository)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, repo := range repos {
		_, err := stmt.Exec(
			repo.GitHubLogin, repo.Name, repo.OwnerLogin, repo.Description,
			repo.URL, repo.SSHURL, repo.HomepageURL, repo.DiskUsage,
			repo.StargazerCount, repo.ForkCount, repo.CommitCount, repo.IsFork, repo.IsEmpty,
			repo.IsInOrganization, repo.HasWikiEnabled, repo.Visibility,
			repo.PrimaryLanguage, repo.LicenseName,
			repo.CreatedAt, repo.UpdatedAt, repo.PushedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save user repository %s: %w", repo.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// GetUserRepositories returns all repositories for a GitHub user
func (db *DB) GetUserRepositories(githubLogin string) ([]models.UserRepository, error) {
	rows, err := db.conn.Query(selectUserRepositories, githubLogin)
	if err != nil {
		return nil, fmt.Errorf("failed to query user repositories: %w", err)
	}
	defer rows.Close()

	var repos []models.UserRepository
	for rows.Next() {
		var r models.UserRepository
		var fetchedAt string
		var primaryLang, licenseName sql.NullString
		if err := rows.Scan(
			&r.ID, &r.GitHubLogin, &r.Name, &r.OwnerLogin, &r.Description,
			&r.URL, &r.SSHURL, &r.HomepageURL, &r.DiskUsage,
			&r.StargazerCount, &r.ForkCount, &r.CommitCount, &r.IsFork, &r.IsEmpty,
			&r.IsInOrganization, &r.HasWikiEnabled, &r.Visibility,
			&primaryLang, &licenseName,
			&r.CreatedAt, &r.UpdatedAt, &r.PushedAt, &fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user repository: %w", err)
		}
		r.FetchedAt, _ = parseTimestamp(fetchedAt)
		r.PrimaryLanguage = primaryLang.String
		r.LicenseName = licenseName.String
		repos = append(repos, r)
	}
	return repos, nil
}

// GetUserRepositoryCount returns the count of repositories for a user
func (db *DB) GetUserRepositoryCount(githubLogin string) (int, error) {
	var count int
	err := db.conn.QueryRow(selectUserRepositoryCount, githubLogin).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get user repository count: %w", err)
	}
	return count, nil
}

// DeleteUserRepositories removes all repositories for a user
func (db *DB) DeleteUserRepositories(githubLogin string) error {
	_, err := db.conn.Exec(deleteUserRepositories, githubLogin)
	if err != nil {
		return fmt.Errorf("failed to delete user repositories: %w", err)
	}
	return nil
}

// DeleteUserRepository removes a single repository for a user
func (db *DB) DeleteUserRepository(githubLogin, repoName string) error {
	_, err := db.conn.Exec(deleteUserRepository, githubLogin, repoName)
	if err != nil {
		return fmt.Errorf("failed to delete user repository: %w", err)
	}
	return nil
}

// DeleteCommitterByEmail removes all commits for a committer by email
func (db *DB) DeleteCommitterByEmail(repoOwner, repoName, email string) error {
	_, err := db.conn.Exec(deleteCommitsByEmail, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to delete committer commits: %w", err)
	}
	// Also clean up tags and links for this email
	db.conn.Exec(deleteTag, repoOwner, repoName, email)
	db.conn.Exec(deleteLink, repoOwner, repoName, email)
	return nil
}

// UpdateCommitterLogin updates the GitHub login for a committer
func (db *DB) UpdateCommitterLogin(repoOwner, repoName, email, login string) error {
	_, err := db.conn.Exec(updateCommitterLogin, login, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to update committer login: %w", err)
	}
	return nil
}

// UpdateCommitterName updates the name for a committer
func (db *DB) UpdateCommitterName(repoOwner, repoName, email, name string) error {
	_, err := db.conn.Exec(updateCommitterName, name, repoOwner, repoName, email)
	if err != nil {
		return fmt.Errorf("failed to update committer name: %w", err)
	}
	return nil
}

// SaveUserGist saves a user's gist to the database
func (db *DB) SaveUserGist(gist models.UserGist) error {
	_, err := db.conn.Exec(insertUserGist,
		gist.ID, gist.GitHubLogin, gist.Name, gist.Description,
		gist.URL, gist.ResourcePath, gist.IsPublic, gist.IsFork,
		gist.StargazerCount, gist.ForkCount, gist.RevisionCount, gist.CreatedAt, gist.UpdatedAt, gist.PushedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save user gist: %w", err)
	}
	return nil
}

// SaveUserGists saves multiple user gists in a transaction
func (db *DB) SaveUserGists(gists []models.UserGist) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	gistStmt, err := tx.Prepare(insertUserGist)
	if err != nil {
		return fmt.Errorf("failed to prepare gist statement: %w", err)
	}
	defer gistStmt.Close()

	fileStmt, err := tx.Prepare(insertGistFile)
	if err != nil {
		return fmt.Errorf("failed to prepare file statement: %w", err)
	}
	defer fileStmt.Close()

	commentStmt, err := tx.Prepare(insertGistComment)
	if err != nil {
		return fmt.Errorf("failed to prepare comment statement: %w", err)
	}
	defer commentStmt.Close()

	for _, gist := range gists {
		_, err := gistStmt.Exec(
			gist.ID, gist.GitHubLogin, gist.Name, gist.Description,
			gist.URL, gist.ResourcePath, gist.IsPublic, gist.IsFork,
			gist.StargazerCount, gist.ForkCount, gist.RevisionCount, gist.CreatedAt, gist.UpdatedAt, gist.PushedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save gist %s: %w", gist.ID, err)
		}

		// Delete existing files before inserting new ones
		_, err = tx.Exec(deleteGistFiles, gist.ID)
		if err != nil {
			return fmt.Errorf("failed to delete existing gist files for %s: %w", gist.ID, err)
		}

		// Save files
		for _, file := range gist.Files {
			_, err := fileStmt.Exec(
				gist.ID, file.Name, file.EncodedName, file.Extension,
				file.Language, file.Size, file.Encoding, file.IsImage, file.IsTruncated, file.Text,
			)
			if err != nil {
				return fmt.Errorf("failed to save gist file %s: %w", file.Name, err)
			}
		}

		// Save comments
		for _, comment := range gist.Comments {
			_, err := commentStmt.Exec(
				comment.ID, gist.ID, comment.AuthorLogin, comment.BodyText,
				comment.CreatedAt, comment.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to save gist comment %s: %w", comment.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// GetUserGists returns all gists for a GitHub user
func (db *DB) GetUserGists(githubLogin string) ([]models.UserGist, error) {
	rows, err := db.conn.Query(selectUserGists, githubLogin)
	if err != nil {
		return nil, fmt.Errorf("failed to query user gists: %w", err)
	}
	defer rows.Close()

	var gists []models.UserGist
	for rows.Next() {
		var g models.UserGist
		var fetchedAt string
		if err := rows.Scan(
			&g.ID, &g.GitHubLogin, &g.Name, &g.Description,
			&g.URL, &g.ResourcePath, &g.IsPublic, &g.IsFork,
			&g.StargazerCount, &g.ForkCount, &g.RevisionCount, &g.CreatedAt, &g.UpdatedAt, &g.PushedAt, &fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user gist: %w", err)
		}
		g.FetchedAt, _ = parseTimestamp(fetchedAt)
		gists = append(gists, g)
	}
	return gists, nil
}

// GetUserGistCount returns the count of gists for a user
func (db *DB) GetUserGistCount(githubLogin string) (int, error) {
	var count int
	err := db.conn.QueryRow(selectUserGistCount, githubLogin).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get user gist count: %w", err)
	}
	return count, nil
}

// DeleteUserGists removes all gists for a user (cascade deletes files and comments)
func (db *DB) DeleteUserGists(githubLogin string) error {
	// First get all gist IDs for this user to delete files and comments
	rows, err := db.conn.Query("SELECT id FROM user_gists WHERE github_login = ?", githubLogin)
	if err != nil {
		return fmt.Errorf("failed to query gist IDs: %w", err)
	}
	defer rows.Close()

	var gistIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan gist ID: %w", err)
		}
		gistIDs = append(gistIDs, id)
	}

	// Delete files and comments for each gist
	for _, gistID := range gistIDs {
		if _, err := db.conn.Exec(deleteGistFiles, gistID); err != nil {
			return fmt.Errorf("failed to delete gist files: %w", err)
		}
		if _, err := db.conn.Exec(deleteGistComments, gistID); err != nil {
			return fmt.Errorf("failed to delete gist comments: %w", err)
		}
	}

	// Delete gists
	_, err = db.conn.Exec(deleteUserGists, githubLogin)
	if err != nil {
		return fmt.Errorf("failed to delete user gists: %w", err)
	}
	return nil
}

// DeleteUserGist removes a single gist and its files/comments
func (db *DB) DeleteUserGist(githubLogin, gistID string) error {
	// Delete files and comments for this gist
	if _, err := db.conn.Exec(deleteGistFiles, gistID); err != nil {
		return fmt.Errorf("failed to delete gist files: %w", err)
	}
	if _, err := db.conn.Exec(deleteGistComments, gistID); err != nil {
		return fmt.Errorf("failed to delete gist comments: %w", err)
	}

	// Delete the gist
	_, err := db.conn.Exec(deleteUserGist, githubLogin, gistID)
	if err != nil {
		return fmt.Errorf("failed to delete user gist: %w", err)
	}
	return nil
}

// GetGistFiles returns all files for a gist
func (db *DB) GetGistFiles(gistID string) ([]models.GistFile, error) {
	rows, err := db.conn.Query(selectGistFiles, gistID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gist files: %w", err)
	}
	defer rows.Close()

	var files []models.GistFile
	for rows.Next() {
		var f models.GistFile
		if err := rows.Scan(
			&f.ID, &f.GistID, &f.Name, &f.EncodedName, &f.Extension,
			&f.Language, &f.Size, &f.Encoding, &f.IsImage, &f.IsTruncated, &f.Text,
		); err != nil {
			return nil, fmt.Errorf("failed to scan gist file: %w", err)
		}
		files = append(files, f)
	}
	return files, nil
}

// GetGistComments returns all comments for a gist
func (db *DB) GetGistComments(gistID string) ([]models.GistComment, error) {
	rows, err := db.conn.Query(selectGistComments, gistID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gist comments: %w", err)
	}
	defer rows.Close()

	var comments []models.GistComment
	for rows.Next() {
		var c models.GistComment
		if err := rows.Scan(
			&c.ID, &c.GistID, &c.AuthorLogin, &c.BodyText, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan gist comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// SaveUserProfile saves a user's profile to the database
func (db *DB) SaveUserProfile(profile models.UserProfile) error {
	// Serialize organizations to JSON
	orgsJSON := "[]"
	if len(profile.Organizations) > 0 {
		bytes, err := json.Marshal(profile.Organizations)
		if err != nil {
			return fmt.Errorf("failed to marshal organizations: %w", err)
		}
		orgsJSON = string(bytes)
	}

	// Serialize social accounts to JSON
	socialJSON := "[]"
	if len(profile.SocialAccounts) > 0 {
		bytes, err := json.Marshal(profile.SocialAccounts)
		if err != nil {
			return fmt.Errorf("failed to marshal social accounts: %w", err)
		}
		socialJSON = string(bytes)
	}

	_, err := db.conn.Exec(insertUserProfile,
		profile.Login, profile.Name, profile.Bio, profile.Company, profile.Location,
		profile.Email, profile.WebsiteURL, profile.TwitterUsername, profile.Pronouns,
		profile.AvatarURL, profile.FollowerCount, profile.FollowingCount,
		profile.CreatedAt, orgsJSON, socialJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to save user profile: %w", err)
	}
	return nil
}

// GetUserProfile retrieves a user's profile from the database
func (db *DB) GetUserProfile(login string) (models.UserProfile, error) {
	var p models.UserProfile
	var orgsJSON, socialJSON sql.NullString
	var fetchedAt string
	var name, bio, company, location, email, websiteURL, twitterUsername, pronouns, avatarURL, createdAt sql.NullString
	var followerCount, followingCount sql.NullInt64

	err := db.conn.QueryRow(selectUserProfile, login).Scan(
		&p.Login, &name, &bio, &company, &location, &email, &websiteURL,
		&twitterUsername, &pronouns, &avatarURL, &followerCount, &followingCount,
		&createdAt, &orgsJSON, &socialJSON, &fetchedAt,
	)
	if err == sql.ErrNoRows {
		return p, nil // Return empty profile if not found
	}
	if err != nil {
		return p, fmt.Errorf("failed to get user profile: %w", err)
	}

	// Map nullable fields
	p.Name = name.String
	p.Bio = bio.String
	p.Company = company.String
	p.Location = location.String
	p.Email = email.String
	p.WebsiteURL = websiteURL.String
	p.TwitterUsername = twitterUsername.String
	p.Pronouns = pronouns.String
	p.AvatarURL = avatarURL.String
	p.CreatedAt = createdAt.String
	p.FollowerCount = int(followerCount.Int64)
	p.FollowingCount = int(followingCount.Int64)

	// Parse organizations from JSON
	if orgsJSON.Valid && orgsJSON.String != "" && orgsJSON.String != "[]" {
		if err := json.Unmarshal([]byte(orgsJSON.String), &p.Organizations); err != nil {
			// Log but don't fail - organizations are optional
			p.Organizations = nil
		}
	}

	// Parse social accounts from JSON
	if socialJSON.Valid && socialJSON.String != "" && socialJSON.String != "[]" {
		if err := json.Unmarshal([]byte(socialJSON.String), &p.SocialAccounts); err != nil {
			// Log but don't fail - social accounts are optional
			p.SocialAccounts = nil
		}
	}

	return p, nil
}

// UserHasData checks if a user has any fetched repositories, gists, or profile
func (db *DB) UserHasData(githubLogin string) (bool, error) {
	var hasData bool
	err := db.conn.QueryRow(selectUserHasData, githubLogin, githubLogin, githubLogin).Scan(&hasData)
	if err != nil {
		return false, fmt.Errorf("failed to check user data: %w", err)
	}
	return hasData, nil
}

// GetTaggedUsersWithLogins returns tagged users that have GitHub logins for the current repo
func (db *DB) GetTaggedUsersWithLogins(repoOwner, repoName string) ([]models.ContributorStats, error) {
	// Join tags with commits to get users with GitHub logins
	query := `
		SELECT DISTINCT c.committer_name, c.committer_email, COALESCE(c.github_committer_login, '') as github_login
		FROM committer_tags t
		JOIN commits c ON t.committer_email = c.committer_email
		WHERE t.repo_owner = ? AND t.repo_name = ?
		AND c.github_committer_login IS NOT NULL AND c.github_committer_login != ''
	`
	rows, err := db.conn.Query(query, repoOwner, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to query tagged users: %w", err)
	}
	defer rows.Close()

	var users []models.ContributorStats
	for rows.Next() {
		var u models.ContributorStats
		if err := rows.Scan(&u.Name, &u.Email, &u.GitHubLogin); err != nil {
			return nil, fmt.Errorf("failed to scan tagged user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

// SaveAPILog saves an API call log entry to the database
func (db *DB) SaveAPILog(method, endpoint string, statusCode int, errorMsg string, rateLimitRemaining int, rateLimitReset, login string) error {
	_, err := db.conn.Exec(insertAPILog, method, endpoint, statusCode, errorMsg, rateLimitRemaining, rateLimitReset, login)
	if err != nil {
		return fmt.Errorf("failed to save API log: %w", err)
	}
	return nil
}

// GetAPILogs returns the most recent API logs
func (db *DB) GetAPILogs(limit int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(selectAPILogs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query API logs: %w", err)
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, statusCode, rateLimitRemaining sql.NullInt64
		var timestamp, method, endpoint, errorMsg, rateLimitReset, login sql.NullString
		
		if err := rows.Scan(&id, &timestamp, &method, &endpoint, &statusCode, &errorMsg, &rateLimitRemaining, &rateLimitReset, &login); err != nil {
			return nil, fmt.Errorf("failed to scan API log: %w", err)
		}
		
		log := map[string]interface{}{
			"id":                   id.Int64,
			"timestamp":            timestamp.String,
			"method":               method.String,
			"endpoint":             endpoint.String,
			"status_code":          statusCode.Int64,
			"error":                errorMsg.String,
			"rate_limit_remaining": rateLimitRemaining.Int64,
			"rate_limit_reset":     rateLimitReset.String,
			"login":                login.String,
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// GetAPILogsByLogin returns API logs for a specific user
func (db *DB) GetAPILogsByLogin(login string, limit int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(selectAPILogsByLogin, login, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query API logs by login: %w", err)
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, statusCode, rateLimitRemaining sql.NullInt64
		var timestamp, method, endpoint, errorMsg, rateLimitReset, loginVal sql.NullString
		
		if err := rows.Scan(&id, &timestamp, &method, &endpoint, &statusCode, &errorMsg, &rateLimitRemaining, &rateLimitReset, &loginVal); err != nil {
			return nil, fmt.Errorf("failed to scan API log: %w", err)
		}
		
		log := map[string]interface{}{
			"id":                   id.Int64,
			"timestamp":            timestamp.String,
			"method":               method.String,
			"endpoint":             endpoint.String,
			"status_code":          statusCode.Int64,
			"error":                errorMsg.String,
			"rate_limit_remaining": rateLimitRemaining.Int64,
			"rate_limit_reset":     rateLimitReset.String,
			"login":                loginVal.String,
		}
		logs = append(logs, log)
	}
	return logs, nil
}

