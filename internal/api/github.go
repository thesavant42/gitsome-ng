package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/models"
)

const (
	baseURL            = "https://api.github.com"
	perPage            = 100 // Max allowed by GitHub API
	userAgent          = "charming-commits/1.0"
	dockerHubUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	dockerHubReferer   = "https://github.com/"
)

// Client is a GitHub API client
type Client struct {
	httpClient *http.Client
	token      string // Optional: for authenticated requests (higher rate limits)
	logger     *log.Logger
}

// NewClient creates a new GitHub API client with a 30 second timeout
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// NewClientWithLogging creates a new GitHub API client with logging enabled
func NewClientWithLogging(token string, dbPath string) *Client {
	// Create logger that writes to file in same directory as database
	logDir := filepath.Dir(dbPath)
	logFile := filepath.Join(logDir, "api.log")

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Fall back to client without file logging if we can't open the log file
		return NewClient(token)
	}

	logger := log.NewWithOptions(f, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
		Prefix:          "API",
	})

	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:  token,
		logger: logger,
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
		if c.logger != nil {
			c.logger.Error("Failed to create request", "url", url, "error", err)
		}
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	if c.logger != nil {
		c.logger.Info("GET", "endpoint", url)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("Request failed", "url", url, "error", err)
		}
		return nil, "", fmt.Errorf("failed to fetch commits: %w", err)
	}
	defer resp.Body.Close()

	// Log rate limit info
	if c.logger != nil {
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		reset := resp.Header.Get("X-RateLimit-Reset")
		c.logger.Debug("Rate limit", "remaining", remaining, "reset", reset, "status", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if c.logger != nil {
			c.logger.Error("API error", "status", resp.StatusCode, "response", string(body))
		}
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

// sanitizeInput removes null bytes and other invisible control characters from input
func sanitizeInput(s string) string {
	// Remove null bytes and other control characters (except whitespace)
	result := strings.Map(func(r rune) rune {
		// Keep printable characters and normal whitespace (space, tab, newline)
		if r == 0 || (r < 32 && r != '\t' && r != '\n' && r != '\r') {
			return -1 // Remove the character
		}
		return r
	}, s)
	return result
}

// ParseRepoString parses "owner/repo" format into owner and repo
func ParseRepoString(repoStr string) (owner, repo string, err error) {
	// Sanitize input to remove null bytes and control characters
	repoStr = sanitizeInput(repoStr)
	parts := strings.Split(strings.TrimSpace(repoStr), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format: expected 'owner/repo', got '%s'", repoStr)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

// ParseCommitsFromJSON parses commits from JSON bytes (for loading from files)
func ParseCommitsFromJSON(data []byte) ([]models.Commit, error) {
	var commits []models.Commit
	if err := json.Unmarshal(data, &commits); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return commits, nil
}

// GraphQL endpoint
const graphQLURL = "https://api.github.com/graphql"

// graphQLRequest represents a GraphQL request
type graphQLRequest struct {
	Query string `json:"query"`
}

// FetchUserReposAndGists fetches all repositories and gists for a GitHub user
func (c *Client) FetchUserReposAndGists(login string) (*models.UserData, error) {
	if c.token == "" {
		return nil, fmt.Errorf("GitHub token required for GraphQL queries")
	}

	query := buildUserDataQuery(login)

	reqBody := graphQLRequest{Query: query}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", graphQLURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		if c.logger != nil {
			c.logger.Error("Failed to create GraphQL request", "login", login, "error", err)
		}
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	if c.logger != nil {
		c.logger.Info("POST GraphQL", "endpoint", graphQLURL, "login", login)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("GraphQL request failed", "login", login, "error", err)
		}
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Log rate limit info
	if c.logger != nil {
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		reset := resp.Header.Get("X-RateLimit-Reset")
		c.logger.Debug("Rate limit", "remaining", remaining, "reset", reset, "status", resp.StatusCode, "login", login)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("Failed to read response", "login", login, "error", err)
		}
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if c.logger != nil {
			c.logger.Error("GraphQL API error", "status", resp.StatusCode, "login", login, "response", string(body))
		}
		return nil, fmt.Errorf("GraphQL error (status %d): %s", resp.StatusCode, string(body))
	}

	// Debug: Log raw response to inspect organizations data
	if c.logger != nil {
		c.logger.Debug("GraphQL response received", "login", login, "bodyLength", len(body))
		// Log just the organizations part if we can parse it
		var tempResp struct {
			Data struct {
				User *struct {
					Login         string `json:"login"`
					Organizations struct {
						Nodes []struct {
							Login string `json:"login"`
							Name  string `json:"name"`
						} `json:"nodes"`
					} `json:"organizations"`
				} `json:"user"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &tempResp); err == nil && tempResp.Data.User != nil {
			c.logger.Debug("Organizations in response", "login", login, "orgCount", len(tempResp.Data.User.Organizations.Nodes), "orgs", tempResp.Data.User.Organizations.Nodes)
		}
	}

	userData, err := parseUserDataResponse(body, login)
	if err != nil {
		return nil, err
	}

	// Check for Docker Hub profile - always add entry, even if not found
	if c.logger != nil {
		c.logger.Info("Checking Docker Hub profile", "login", login)
	}
	hasDockerHub, err := CheckDockerHubProfile(login)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("Docker Hub check failed", "login", login, "error", err)
		}
		// Add entry showing check failed
		userData.Profile.SocialAccounts = append(userData.Profile.SocialAccounts, models.SocialAccount{
			Provider:    "DOCKERHUB",
			DisplayName: "Error",
			URL:         fmt.Sprintf("https://hub.docker.com/u/%s", login),
		})
	} else if hasDockerHub {
		userData.Profile.SocialAccounts = append(userData.Profile.SocialAccounts, models.SocialAccount{
			Provider:    "DOCKERHUB",
			DisplayName: login,
			URL:         fmt.Sprintf("https://hub.docker.com/u/%s", login),
		})
		if c.logger != nil {
			c.logger.Info("Docker Hub profile found", "login", login)
		}
	} else {
		// Profile doesn't exist - still add entry showing "None"
		userData.Profile.SocialAccounts = append(userData.Profile.SocialAccounts, models.SocialAccount{
			Provider:    "DOCKERHUB",
			DisplayName: "None",
			URL:         fmt.Sprintf("https://hub.docker.com/u/%s", login),
		})
		if c.logger != nil {
			c.logger.Info("No Docker Hub profile found", "login", login)
		}
	}

	// Add small delay after Docker Hub check to be respectful
	time.Sleep(100 * time.Millisecond)

	return userData, nil
}

// buildUserDataQuery constructs the GraphQL query for user data
func buildUserDataQuery(login string) string {
	return fmt.Sprintf(`
query {
  user(login: "%s") {
    login
    name
    bio
    company
    location
    email
    websiteUrl
    twitterUsername
    pronouns
    avatarUrl
    followers { totalCount }
    following { totalCount }
    createdAt
    socialAccounts(first: 10) {
      nodes {
        provider
        displayName
        url
      }
    }
    organizations(first: 100) {
      nodes {
        login
        name
      }
    }
    repositories(first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
      totalCount
      nodes {
        name
        owner { login }
        description
        url
        sshUrl
        homepageUrl
        diskUsage
        stargazerCount
        forkCount
        isFork
        isEmpty
        isInOrganization
        hasWikiEnabled
        visibility
        primaryLanguage { name }
        licenseInfo { name }
        createdAt
        updatedAt
        pushedAt
      }
    }
    gists(first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
      totalCount
      nodes {
        id
        name
        description
        url
        resourcePath
        isPublic
        isFork
        stargazerCount
        forks { totalCount }
        createdAt
        updatedAt
        pushedAt
        files {
          name
          encodedName
          extension
          language { name }
          size
          encoding
          isImage
          isTruncated
          text
        }
        comments(first: 100) {
          nodes {
            id
            author { login }
            bodyText
            createdAt
            updatedAt
          }
        }
      }
    }
  }
}
`, login)
}

// graphQLResponse represents the structure of the GraphQL response
type graphQLResponse struct {
	Data struct {
		User *graphQLUser `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type graphQLUser struct {
	Login           string `json:"login"`
	Name            string `json:"name"`
	Bio             string `json:"bio"`
	Company         string `json:"company"`
	Location        string `json:"location"`
	Email           string `json:"email"`
	WebsiteUrl      string `json:"websiteUrl"`
	TwitterUsername string `json:"twitterUsername"`
	Pronouns        string `json:"pronouns"`
	AvatarUrl       string `json:"avatarUrl"`
	Followers       struct {
		TotalCount int `json:"totalCount"`
	} `json:"followers"`
	Following struct {
		TotalCount int `json:"totalCount"`
	} `json:"following"`
	CreatedAt      string `json:"createdAt"`
	SocialAccounts struct {
		Nodes []graphQLSocialAccount `json:"nodes"`
	} `json:"socialAccounts"`
	Organizations struct {
		Nodes []struct {
			Login string `json:"login"`
			Name  string `json:"name"`
		} `json:"nodes"`
	} `json:"organizations"`
	Repositories struct {
		TotalCount int           `json:"totalCount"`
		Nodes      []graphQLRepo `json:"nodes"`
	} `json:"repositories"`
	Gists struct {
		TotalCount int           `json:"totalCount"`
		Nodes      []graphQLGist `json:"nodes"`
	} `json:"gists"`
}

type graphQLSocialAccount struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
}

type graphQLRepo struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description      string `json:"description"`
	URL              string `json:"url"`
	SSHUrl           string `json:"sshUrl"`
	HomepageUrl      string `json:"homepageUrl"`
	DiskUsage        int    `json:"diskUsage"`
	StargazerCount   int    `json:"stargazerCount"`
	ForkCount        int    `json:"forkCount"`
	IsFork           bool   `json:"isFork"`
	IsEmpty          bool   `json:"isEmpty"`
	IsInOrganization bool   `json:"isInOrganization"`
	HasWikiEnabled   bool   `json:"hasWikiEnabled"`
	Visibility       string `json:"visibility"`
	PrimaryLanguage  *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
	LicenseInfo *struct {
		Name string `json:"name"`
	} `json:"licenseInfo"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	PushedAt         string `json:"pushedAt"`
	DefaultBranchRef *struct {
		Target struct {
			History *struct {
				TotalCount int `json:"totalCount"`
			} `json:"history"`
		} `json:"target"`
	} `json:"defaultBranchRef"`
}

type graphQLGist struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	URL            string `json:"url"`
	ResourcePath   string `json:"resourcePath"`
	IsPublic       bool   `json:"isPublic"`
	IsFork         bool   `json:"isFork"`
	StargazerCount int    `json:"stargazerCount"`
	Forks          struct {
		TotalCount int `json:"totalCount"`
	} `json:"forks"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	PushedAt  string `json:"pushedAt"`
	History   struct {
		TotalCount int `json:"totalCount"`
	} `json:"history"`
	Files    []graphQLGistFile `json:"files"`
	Comments struct {
		Nodes []graphQLGistComment `json:"nodes"`
	} `json:"comments"`
}

type graphQLGistFile struct {
	Name        string `json:"name"`
	EncodedName string `json:"encodedName"`
	Extension   string `json:"extension"`
	Language    *struct {
		Name string `json:"name"`
	} `json:"language"`
	Size        int    `json:"size"`
	Encoding    string `json:"encoding"`
	IsImage     bool   `json:"isImage"`
	IsTruncated bool   `json:"isTruncated"`
	Text        string `json:"text"`
}

type graphQLGistComment struct {
	ID     string `json:"id"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	BodyText  string `json:"bodyText"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// parseUserDataResponse parses the GraphQL response into UserData
func parseUserDataResponse(body []byte, login string) (*models.UserData, error) {
	var response graphQLResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", response.Errors[0].Message)
	}

	if response.Data.User == nil {
		return nil, fmt.Errorf("user not found: %s", login)
	}

	user := response.Data.User

	// Build profile
	profile := models.UserProfile{
		Login:           user.Login,
		Name:            user.Name,
		Bio:             user.Bio,
		Company:         user.Company,
		Location:        user.Location,
		Email:           user.Email,
		WebsiteURL:      user.WebsiteUrl,
		TwitterUsername: user.TwitterUsername,
		Pronouns:        user.Pronouns,
		AvatarURL:       user.AvatarUrl,
		FollowerCount:   user.Followers.TotalCount,
		FollowingCount:  user.Following.TotalCount,
		CreatedAt:       user.CreatedAt,
	}
	for _, sa := range user.SocialAccounts.Nodes {
		profile.SocialAccounts = append(profile.SocialAccounts, models.SocialAccount{
			Provider:    sa.Provider,
			DisplayName: sa.DisplayName,
			URL:         sa.URL,
		})
	}

	// Populate organizations
	// Note: The GraphQL API's 'organizations' field on User only returns
	// PUBLIC organization memberships. Private memberships are not included.
	// Users must explicitly make their org membership public in their GitHub settings.
	for _, org := range user.Organizations.Nodes {
		profile.Organizations = append(profile.Organizations, org.Login)
	}

	userData := &models.UserData{
		Login:   user.Login,
		Profile: profile,
	}

	// Convert repositories
	for _, repo := range user.Repositories.Nodes {
		commitCount := 0
		if repo.DefaultBranchRef != nil && repo.DefaultBranchRef.Target.History != nil {
			commitCount = repo.DefaultBranchRef.Target.History.TotalCount
		}
		primaryLang := ""
		if repo.PrimaryLanguage != nil {
			primaryLang = repo.PrimaryLanguage.Name
		}
		licenseName := ""
		if repo.LicenseInfo != nil {
			licenseName = repo.LicenseInfo.Name
		}
		userData.Repositories = append(userData.Repositories, models.UserRepository{
			GitHubLogin:      login,
			Name:             repo.Name,
			OwnerLogin:       repo.Owner.Login,
			Description:      repo.Description,
			URL:              repo.URL,
			SSHURL:           repo.SSHUrl,
			HomepageURL:      repo.HomepageUrl,
			DiskUsage:        repo.DiskUsage,
			StargazerCount:   repo.StargazerCount,
			ForkCount:        repo.ForkCount,
			CommitCount:      commitCount,
			IsFork:           repo.IsFork,
			IsEmpty:          repo.IsEmpty,
			IsInOrganization: repo.IsInOrganization,
			HasWikiEnabled:   repo.HasWikiEnabled,
			Visibility:       repo.Visibility,
			PrimaryLanguage:  primaryLang,
			LicenseName:      licenseName,
			CreatedAt:        repo.CreatedAt,
			UpdatedAt:        repo.UpdatedAt,
			PushedAt:         repo.PushedAt,
		})
	}

	// Convert gists
	for _, gist := range user.Gists.Nodes {
		userGist := models.UserGist{
			ID:             gist.ID,
			GitHubLogin:    login,
			Name:           gist.Name,
			Description:    gist.Description,
			URL:            gist.URL,
			ResourcePath:   gist.ResourcePath,
			IsPublic:       gist.IsPublic,
			IsFork:         gist.IsFork,
			StargazerCount: gist.StargazerCount,
			ForkCount:      gist.Forks.TotalCount,
			RevisionCount:  gist.History.TotalCount,
			CreatedAt:      gist.CreatedAt,
			UpdatedAt:      gist.UpdatedAt,
			PushedAt:       gist.PushedAt,
		}

		// Convert files
		for _, file := range gist.Files {
			lang := ""
			if file.Language != nil {
				lang = file.Language.Name
			}
			userGist.Files = append(userGist.Files, models.GistFile{
				GistID:      gist.ID,
				Name:        file.Name,
				EncodedName: file.EncodedName,
				Extension:   file.Extension,
				Language:    lang,
				Size:        file.Size,
				Encoding:    file.Encoding,
				IsImage:     file.IsImage,
				IsTruncated: file.IsTruncated,
				Text:        file.Text,
			})
		}

		// Convert comments
		for _, comment := range gist.Comments.Nodes {
			authorLogin := ""
			if comment.Author != nil {
				authorLogin = comment.Author.Login
			}
			userGist.Comments = append(userGist.Comments, models.GistComment{
				ID:          comment.ID,
				GistID:      gist.ID,
				AuthorLogin: authorLogin,
				BodyText:    comment.BodyText,
				CreatedAt:   comment.CreatedAt,
				UpdatedAt:   comment.UpdatedAt,
			})
		}

		userData.Gists = append(userData.Gists, userGist)
	}

	return userData, nil
}

// CheckDockerHubProfile checks if a Docker Hub profile exists for the given username
// Returns true if the profile exists (status 200), false if not found (status 404)
func CheckDockerHubProfile(username string) (bool, error) {
	url := fmt.Sprintf("https://hub.docker.com/u/%s", username)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Set spoofed headers to appear as browser traffic from GitHub
	req.Header.Set("User-Agent", dockerHubUserAgent)
	req.Header.Set("Referer", dockerHubReferer)

	client := &http.Client{
		Timeout: 10 * time.Second,
		// Don't follow redirects - we only care about the initial response
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check Docker Hub profile: %w", err)
	}
	defer resp.Body.Close()

	// 200 = profile exists, 404 = profile doesn't exist
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		// For any other status code, treat as error
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}
