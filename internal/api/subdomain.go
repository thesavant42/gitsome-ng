package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/models"
)

const (
	vtAPIBaseURL     = "https://www.virustotal.com/api/v3"
	crtshBaseURL     = "https://crt.sh"
	subdomainTimeout = 60 * time.Second
	vtBatchSize      = 40 // VT API default limit
)

// SubdomainClient handles subdomain enumeration API requests
type SubdomainClient struct {
	httpClient *http.Client
	vtAPIKey   string
	logger     *log.Logger
}

// NewSubdomainClient creates a new subdomain enumeration client
func NewSubdomainClient(vtAPIKey string, logger *log.Logger) *SubdomainClient {
	return &SubdomainClient{
		httpClient: &http.Client{
			Timeout: subdomainTimeout,
		},
		vtAPIKey: vtAPIKey,
		logger:   logger,
	}
}

// SetVirusTotalAPIKey updates the VirusTotal API key
func (c *SubdomainClient) SetVirusTotalAPIKey(apiKey string) {
	c.vtAPIKey = apiKey
}

// HasVirusTotalAPIKey returns true if an API key is configured
func (c *SubdomainClient) HasVirusTotalAPIKey() bool {
	return c.vtAPIKey != ""
}

// =============================================================================
// VirusTotal API
// =============================================================================

// FetchVirusTotalSubdomains fetches subdomains from VirusTotal API
// Returns subdomains, cursor for next page, and any error
func (c *SubdomainClient) FetchVirusTotalSubdomains(domain string, cursor string) ([]models.Subdomain, string, error) {
	if c.vtAPIKey == "" {
		return nil, "", fmt.Errorf("VirusTotal API key not configured")
	}

	// Build URL
	reqURL := fmt.Sprintf("%s/domains/%s/subdomains?limit=%d", vtAPIBaseURL, url.PathEscape(domain), vtBatchSize)
	if cursor != "" {
		reqURL += "&cursor=" + url.QueryEscape(cursor)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-apikey", c.vtAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, "", fmt.Errorf("invalid API key")
	}
	if resp.StatusCode == 429 {
		return nil, "", fmt.Errorf("rate limited - please wait and try again")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var vtResp models.VirusTotalSubdomainResponse
	if err := json.Unmarshal(body, &vtResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to Subdomain structs
	subdomains := make([]models.Subdomain, 0, len(vtResp.Data))
	for _, item := range vtResp.Data {
		if item.Type == "domain" && item.ID != "" {
			subdomains = append(subdomains, models.Subdomain{
				Domain:    domain,
				Subdomain: item.ID,
				Source:    "virustotal",
			})
		}
	}

	// Get next cursor if available
	nextCursor := vtResp.Meta.Cursor

	return subdomains, nextCursor, nil
}

// FetchAllVirusTotalSubdomains fetches all subdomains from VirusTotal with pagination
func (c *SubdomainClient) FetchAllVirusTotalSubdomains(domain string, progress func(count int), cancel <-chan struct{}) ([]models.Subdomain, error) {
	var allSubdomains []models.Subdomain
	cursor := ""

	for {
		// Check for cancellation
		select {
		case <-cancel:
			return allSubdomains, fmt.Errorf("cancelled")
		default:
		}

		batch, nextCursor, err := c.FetchVirusTotalSubdomains(domain, cursor)
		if err != nil {
			if len(allSubdomains) > 0 {
				// Return partial results on error
				return allSubdomains, err
			}
			return nil, err
		}

		allSubdomains = append(allSubdomains, batch...)

		if progress != nil {
			progress(len(allSubdomains))
		}

		if c.logger != nil {
			c.logger.Info("VT subdomains fetched", "count", len(allSubdomains), "hasMore", nextCursor != "")
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor

		// Small delay to be respectful to the API
		time.Sleep(500 * time.Millisecond)
	}

	return allSubdomains, nil
}

// =============================================================================
// crt.sh API
// =============================================================================

// FetchCrtshSubdomains fetches subdomains from crt.sh certificate transparency logs
func (c *SubdomainClient) FetchCrtshSubdomains(domain string) ([]models.Subdomain, error) {
	// Build URL - use wildcard query to get all subdomains
	reqURL := fmt.Sprintf("%s/?q=%%.%s&output=json", crtshBaseURL, url.QueryEscape(domain))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("crt.sh returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle empty response
	if len(body) == 0 || string(body) == "[]" || string(body) == "null" {
		return []models.Subdomain{}, nil
	}

	var crtshResp models.CrtshResponse
	if err := json.Unmarshal(body, &crtshResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Process entries and deduplicate
	subdomainMap := make(map[string]*models.Subdomain)
	now := time.Now()

	for _, entry := range crtshResp {
		// Parse certificate expiry
		notAfter, _ := time.Parse("2006-01-02T15:04:05", entry.NotAfter)
		isExpired := !notAfter.IsZero() && notAfter.Before(now)

		// name_value can contain multiple names separated by newlines (SANs)
		names := strings.Split(entry.NameValue, "\n")
		for _, name := range names {
			name = strings.TrimSpace(name)
			name = strings.ToLower(name)

			// Skip empty names and wildcards
			if name == "" || strings.HasPrefix(name, "*") {
				continue
			}

			// Ensure it's a subdomain of the target domain
			if !strings.HasSuffix(name, "."+domain) && name != domain {
				continue
			}

			// Get or create subdomain entry
			existing, ok := subdomainMap[name]
			if !ok {
				subdomainMap[name] = &models.Subdomain{
					Domain:      domain,
					Subdomain:   name,
					Source:      "crtsh",
					CertExpired: isExpired,
				}
			} else {
				// Update expired flag if any cert is expired
				if isExpired {
					existing.CertExpired = true
				}
			}

			// Track common name - also add it as a separate subdomain if it's different
			if entry.CommonName != "" && entry.CommonName != name {
				cn := strings.TrimSpace(strings.ToLower(entry.CommonName))
				// Skip wildcards
				if !strings.HasPrefix(cn, "*") && cn != "" {
					// Add CNAME as its own subdomain entry so it can be researched
					if _, exists := subdomainMap[cn]; !exists {
						subdomainMap[cn] = &models.Subdomain{
							Domain:      domain,
							Subdomain:   cn,
							Source:      "crtsh",
							CertExpired: isExpired,
							AltNames:    "via CN of " + name,
						}
					}
				}
			}
		}
	}

	// Convert map to slice
	subdomains := make([]models.Subdomain, 0, len(subdomainMap))
	for _, sd := range subdomainMap {
		subdomains = append(subdomains, *sd)
	}

	return subdomains, nil
}

// =============================================================================
// JSON Import
// =============================================================================

// ImportVirusTotalJSON parses a VirusTotal API JSON export file
func (c *SubdomainClient) ImportVirusTotalJSON(data []byte, domain string) ([]models.Subdomain, error) {
	var vtResp models.VirusTotalSubdomainResponse
	if err := json.Unmarshal(data, &vtResp); err != nil {
		// Try parsing as array of domains directly
		var domains []string
		if err2 := json.Unmarshal(data, &domains); err2 == nil {
			subdomains := make([]models.Subdomain, 0, len(domains))
			for _, d := range domains {
				subdomains = append(subdomains, models.Subdomain{
					Domain:    domain,
					Subdomain: d,
					Source:    "import",
				})
			}
			return subdomains, nil
		}
		return nil, fmt.Errorf("failed to parse VirusTotal JSON: %w", err)
	}

	subdomains := make([]models.Subdomain, 0, len(vtResp.Data))
	for _, item := range vtResp.Data {
		if item.Type == "domain" && item.ID != "" {
			subdomains = append(subdomains, models.Subdomain{
				Domain:    domain,
				Subdomain: item.ID,
				Source:    "import",
			})
		}
	}

	return subdomains, nil
}

// ImportCrtshJSON parses a crt.sh JSON export file
func (c *SubdomainClient) ImportCrtshJSON(data []byte, domain string) ([]models.Subdomain, error) {
	var crtshResp models.CrtshResponse
	if err := json.Unmarshal(data, &crtshResp); err != nil {
		return nil, fmt.Errorf("failed to parse crt.sh JSON: %w", err)
	}

	// Reuse the same processing logic as live fetch
	subdomainMap := make(map[string]*models.Subdomain)
	now := time.Now()

	for _, entry := range crtshResp {
		notAfter, _ := time.Parse("2006-01-02T15:04:05", entry.NotAfter)
		isExpired := !notAfter.IsZero() && notAfter.Before(now)

		names := strings.Split(entry.NameValue, "\n")
		for _, name := range names {
			name = strings.TrimSpace(name)
			name = strings.ToLower(name)

			if name == "" || strings.HasPrefix(name, "*") {
				continue
			}

			if !strings.HasSuffix(name, "."+domain) && name != domain {
				continue
			}

			existing, ok := subdomainMap[name]
			if !ok {
				subdomainMap[name] = &models.Subdomain{
					Domain:      domain,
					Subdomain:   name,
					Source:      "import",
					CertExpired: isExpired,
				}
			} else {
				if isExpired {
					existing.CertExpired = true
				}
			}

			if entry.CommonName != "" && entry.CommonName != name {
				sd := subdomainMap[name]
				if sd.CNAMEs == "" {
					sd.CNAMEs = entry.CommonName
				} else if !strings.Contains(sd.CNAMEs, entry.CommonName) {
					sd.CNAMEs += "," + entry.CommonName
				}
			}
		}
	}

	subdomains := make([]models.Subdomain, 0, len(subdomainMap))
	for _, sd := range subdomainMap {
		subdomains = append(subdomains, *sd)
	}

	return subdomains, nil
}

// ImportPlainTextSubdomains parses a plain text file with one subdomain per line
func (c *SubdomainClient) ImportPlainTextSubdomains(data []byte, domain string) ([]models.Subdomain, error) {
	lines := strings.Split(string(data), "\n")
	subdomainMap := make(map[string]bool)
	var subdomains []models.Subdomain

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.ToLower(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip if already seen
		if subdomainMap[line] {
			continue
		}
		subdomainMap[line] = true

		// Validate it's related to the domain
		if !strings.HasSuffix(line, "."+domain) && line != domain {
			continue
		}

		subdomains = append(subdomains, models.Subdomain{
			Domain:    domain,
			Subdomain: line,
			Source:    "import",
		})
	}

	return subdomains, nil
}

