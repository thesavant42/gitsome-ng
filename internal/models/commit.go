package models

import "time"

// GitUser represents the git identity (name, email, date) from commit.author/commit.committer
type GitUser struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// GitHubUser represents a GitHub account (can be nil for non-GitHub users)
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// CommitDetails contains the git commit metadata
type CommitDetails struct {
	Author       GitUser `json:"author"`
	Committer    GitUser `json:"committer"`
	Message      string  `json:"message"`
	CommentCount int     `json:"comment_count"`
}

// Commit represents a GitHub API commit response
type Commit struct {
	SHA       string         `json:"sha"`
	NodeID    string         `json:"node_id"`
	Commit    CommitDetails  `json:"commit"`
	Author    *GitHubUser    `json:"author"`    // GitHub user, can be null
	Committer *GitHubUser    `json:"committer"` // GitHub user, can be null
	HTMLURL   string         `json:"html_url"`
	Parents   []CommitParent `json:"parents"`
}

// CommitParent represents a parent commit reference
type CommitParent struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
}

// CommitRecord is the flattened structure for database storage
type CommitRecord struct {
	SHA                  string
	Message              string
	AuthorName           string
	AuthorEmail          string
	AuthorDate           time.Time
	CommitterName        string
	CommitterEmail       string
	CommitterDate        time.Time
	GitHubAuthorLogin    string
	GitHubCommitterLogin string
	HTMLURL              string
	RepoOwner            string
	RepoName             string
}

// ToRecord converts a Commit to a CommitRecord for database storage
func (c *Commit) ToRecord(repoOwner, repoName string) CommitRecord {
	record := CommitRecord{
		SHA:           c.SHA,
		Message:       c.Commit.Message,
		AuthorName:    c.Commit.Author.Name,
		AuthorEmail:   c.Commit.Author.Email,
		AuthorDate:    c.Commit.Author.Date,
		CommitterName: c.Commit.Committer.Name,
		CommitterEmail: c.Commit.Committer.Email,
		CommitterDate: c.Commit.Committer.Date,
		HTMLURL:       c.HTMLURL,
		RepoOwner:     repoOwner,
		RepoName:      repoName,
	}

	if c.Author != nil {
		record.GitHubAuthorLogin = c.Author.Login
	}
	if c.Committer != nil {
		record.GitHubCommitterLogin = c.Committer.Login
	}

	return record
}

// ContributorStats holds statistics for a contributor
type ContributorStats struct {
	Name        string
	Email       string
	GitHubLogin string
	CommitCount int
	Percentage  float64
}

// RepoInfo holds information about a tracked repository
type RepoInfo struct {
	Owner   string
	Name    string
	AddedAt time.Time
}

// UserRepository represents a repository owned by a GitHub user
type UserRepository struct {
	ID               int
	GitHubLogin      string
	Name             string
	OwnerLogin       string
	Description      string
	URL              string
	SSHURL           string
	HomepageURL      string
	DiskUsage        int
	StargazerCount   int
	ForkCount        int
	CommitCount      int // Total commits on default branch
	IsFork           bool
	IsEmpty          bool
	IsInOrganization bool
	HasWikiEnabled   bool
	Visibility       string
	CreatedAt        string
	UpdatedAt        string
	PushedAt         string
	FetchedAt        time.Time
}

// UserGist represents a gist owned by a GitHub user
type UserGist struct {
	ID             string
	GitHubLogin    string
	Name           string
	Description    string
	URL            string
	ResourcePath   string
	IsPublic       bool
	IsFork         bool
	StargazerCount int
	RevisionCount  int // Number of revisions/versions
	CreatedAt      string
	UpdatedAt      string
	PushedAt       string
	FetchedAt      time.Time
	Files          []GistFile    // Populated when fetched
	Comments       []GistComment // Populated when fetched
}

// GistFile represents a file within a gist
type GistFile struct {
	ID          int
	GistID      string
	Name        string
	EncodedName string
	Extension   string
	Language    string
	Size        int
	Encoding    string
	IsImage     bool
	IsTruncated bool
	Text        string
}

// GistComment represents a comment on a gist
type GistComment struct {
	ID          string
	GistID      string
	AuthorLogin string
	BodyText    string
	CreatedAt   string
	UpdatedAt   string
}

// UserData contains all fetched data for a user
type UserData struct {
	Login        string
	Repositories []UserRepository
	Gists        []UserGist
}

