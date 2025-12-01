package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/charmbracelet/log"
)

const (
	dockerHubSearchURL       = "https://hub.docker.com/api/search/v3/catalog/search"
	dockerHubPageSize        = 25
	dockerHubSearchUserAgent = "Docker-Client/24.0.0 (linux)"
)

// DockerHubClient handles Docker Hub API requests
type DockerHubClient struct {
	httpClient *http.Client
	logger     *log.Logger
}

// DockerHubSearchResult represents a single search result from Docker Hub
type DockerHubSearchResult struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Slug             string   `json:"slug"`
	Type             string   `json:"type"`
	Publisher        string   `json:"publisher"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	ShortDescription string   `json:"short_description"`
	Badge            string   `json:"badge"` // "verified_publisher", "official", "none"
	StarCount        int      `json:"star_count"`
	PullCount        string   `json:"pull_count"` // e.g., "5M+", "100K+"
	Architectures    []string `json:"architectures"`
	Categories       []string `json:"categories"`
}

// DockerHubSearchResponse represents the full search response
type DockerHubSearchResponse struct {
	Total   int                     `json:"total"`
	Results []DockerHubSearchResult `json:"results"`
	Page    int                     `json:"page"`
	Query   string                  `json:"query"`
}

// NewDockerHubClient creates a new Docker Hub API client
func NewDockerHubClient(logger *log.Logger) *DockerHubClient {
	return &DockerHubClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// Search searches Docker Hub for images matching the query
func (c *DockerHubClient) Search(query string, page int) (*DockerHubSearchResponse, error) {
	if page < 1 {
		page = 1
	}

	// Build the search URL
	params := url.Values{}
	params.Set("query", query)
	params.Set("from", fmt.Sprintf("%d", (page-1)*dockerHubPageSize))
	params.Set("size", fmt.Sprintf("%d", dockerHubPageSize))

	searchURL := fmt.Sprintf("%s?%s", dockerHubSearchURL, params.Encode())

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to appear as a browser
	req.Header.Set("User-Agent", dockerHubSearchUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://hub.docker.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response - Docker Hub returns a complex nested structure
	var rawResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract results from the response
	return c.parseSearchResponse(rawResponse, query, page)
}

// parseSearchResponse extracts search results from Docker Hub's response format
func (c *DockerHubClient) parseSearchResponse(raw map[string]interface{}, query string, page int) (*DockerHubSearchResponse, error) {
	response := &DockerHubSearchResponse{
		Query: query,
		Page:  page,
	}

	// Try to get total count
	if total, ok := raw["total"].(float64); ok {
		response.Total = int(total)
	}

	// Try to get results array
	results, ok := raw["results"].([]interface{})
	if !ok {
		// Docker Hub may return results differently, try alternate paths
		if c.logger != nil {
			c.logger.Debug("Could not find results array in response")
		}
		return response, nil
	}

	for _, item := range results {
		result, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		searchResult := DockerHubSearchResult{}

		// Extract fields with type safety
		if id, ok := result["id"].(string); ok {
			searchResult.ID = id
		}
		if name, ok := result["name"].(string); ok {
			searchResult.Name = name
		}
		if slug, ok := result["slug"].(string); ok {
			searchResult.Slug = slug
		}
		if typ, ok := result["type"].(string); ok {
			searchResult.Type = typ
		}
		if shortDesc, ok := result["short_description"].(string); ok {
			searchResult.ShortDescription = shortDesc
		}
		if badge, ok := result["badge"].(string); ok {
			searchResult.Badge = badge
		}
		if starCount, ok := result["star_count"].(float64); ok {
			searchResult.StarCount = int(starCount)
		}
		if pullCount, ok := result["pull_count"].(string); ok {
			searchResult.PullCount = pullCount
		}

		// Extract publisher from nested object
		if publisher, ok := result["publisher"].(map[string]interface{}); ok {
			if name, ok := publisher["name"].(string); ok {
				searchResult.Publisher = name
			}
		}

		// Extract architectures
		if archs, ok := result["architectures"].([]interface{}); ok {
			for _, arch := range archs {
				if archMap, ok := arch.(map[string]interface{}); ok {
					if name, ok := archMap["name"].(string); ok {
						searchResult.Architectures = append(searchResult.Architectures, name)
					}
				}
			}
		}

		response.Results = append(response.Results, searchResult)
	}

	return response, nil
}

