package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"charming-commits/internal/models"
)

const (
	baseURL    = "https://api.github.com"
	perPage    = 100 // Max allowed by GitHub API
	userAgent  = "charming-commits/1.0"
)

// Client is a GitHub API client
type Client struct {
	httpClient *http.Client
	token      string // Optional: for authenticated requests (higher rate limits)
}

// NewClient creates a new GitHub API client
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{},
		token:      token,
	}
}

// FetchCommits fetches commits from a repository with pagination
// If sinceSHA is provided, only fetches commits newer than that SHA (incremental fetch)
func (c *Client) FetchCommits(owner, repo string, sinceSHA string, onProgress func(fetched, page int)) ([]models.Commit, error) {
	var allCommits []models.Commit
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d", baseURL, owner, repo, perPage)
	page := 1

	for url != "" {
		commits, nextURL, err := c.fetchCommitPage(url)
		if err != nil {
			return nil, err
		}

		// If doing incremental fetch, stop when we hit the known commit
		if sinceSHA != "" {
			for i, commit := range commits {
				if commit.SHA == sinceSHA {
					// Found the last known commit, return only the new ones
					allCommits = append(allCommits, commits[:i]...)
					return allCommits, nil
				}
			}
		}

		allCommits = append(allCommits, commits...)

		if onProgress != nil {
			onProgress(len(allCommits), page)
		}

		url = nextURL
		page++
	}

	return allCommits, nil
}

// fetchCommitPage fetches a single page of commits and returns the next page URL
func (c *Client) fetchCommitPage(url string) ([]models.Commit, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch commits: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	var commits []models.Commit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse Link header for pagination
	nextURL := parseNextLink(resp.Header.Get("Link"))

	return commits, nextURL, nil
}

// parseNextLink extracts the "next" URL from GitHub's Link header
// Example: <https://api.github.com/repos/owner/repo/commits?page=2>; rel="next"
func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Match pattern: <URL>; rel="next"
	re := regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) >= 2 {
		return matches[1]
	}

	return ""
}

// ParseRepoString parses "owner/repo" format into owner and repo
func ParseRepoString(repoStr string) (owner, repo string, err error) {
	parts := strings.Split(strings.TrimSpace(repoStr), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format: expected 'owner/repo', got '%s'", repoStr)
	}
	return parts[0], parts[1], nil
}

// ParseCommitsFromJSON parses commits from JSON bytes (for loading from files)
func ParseCommitsFromJSON(data []byte) ([]models.Commit, error) {
	var commits []models.Commit
	if err := json.Unmarshal(data, &commits); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return commits, nil
}

