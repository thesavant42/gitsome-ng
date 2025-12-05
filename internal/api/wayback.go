package api

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/models"
	"golang.org/x/net/publicsuffix"
)

const (
	cdxTimeout   = 180 * time.Second // 3 minutes for large domain queries
	cdxBatchSize = 1000              // Larger batch size for efficiency (1000 records per request)
	// Note: Larger batches = fewer requests, faster overall
	// Rate limiting is handled by the caller (e.g., TUI's configurable delay)
)

// WaybackClient handles Wayback Machine CDX API requests
type WaybackClient struct {
	httpClient *http.Client
	logger     *log.Logger
}

// NewWaybackClient creates a new Wayback Machine API client
func NewWaybackClient(logger *log.Logger) *WaybackClient {
	return &WaybackClient{
		httpClient: &http.Client{
			Timeout: cdxTimeout,
		},
		logger: logger,
	}
}

// ExtractRootDomain extracts the root domain from a URL or hostname
// Uses publicsuffix to handle complex TLDs like .co.uk
// Examples:
//   - "https://playground.bfl.ai/" -> "bfl.ai"
//   - "test1.dev.pci.westcoast.acme.com" -> "acme.com"
//   - "bfl.ai" -> "bfl.ai"
func ExtractRootDomain(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}

	// If it looks like a URL, parse it
	if strings.Contains(input, "://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		input = parsed.Hostname()
	}

	// Remove any trailing dots
	input = strings.TrimSuffix(input, ".")

	// Use publicsuffix to get the effective TLD+1 (root domain)
	rootDomain, err := publicsuffix.EffectiveTLDPlusOne(input)
	if err != nil {
		return "", fmt.Errorf("failed to extract root domain: %w", err)
	}

	return rootDomain, nil
}

// BuildCDXQuery constructs the raw query string for CDX API
// Returns the query string WITHOUT the leading '?'
// The asterisk wildcard must NOT be URL-encoded for the CDX API
func BuildCDXQuery(domain string, resumeKey string) string {
	// Clean domain: lowercase and trim whitespace
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Build query with domain wildcard - *.domain matches all subdomains
	// Per CDX API docs: *.domain = matchType=domain (all subdomains)
	// Note: Cannot combine *.domain/* - only one wildcard type allowed
	// IMPORTANT: The asterisk must remain literal (not encoded as %2A)
	query := fmt.Sprintf(
		"url=*.%s&output=json&fl=original,timestamp,statuscode,mimetype&collapse=urlkey&limit=%d&showResumeKey=true",
		domain,
		cdxBatchSize,
	)

	if resumeKey != "" {
		query += "&resumeKey=" + url.QueryEscape(resumeKey)
	}

	return query
}

// BuildCDXQueryLatest constructs a query to get the most recent valid record for a URL
// This is much more efficient than fetching all records when you only need the latest accessible one
// Uses limit=-1 to get the last result, and filters for valid responses (200 OK or 3xx redirects)
func BuildCDXQueryLatest(targetURL string) string {
	// Clean URL: trim whitespace
	targetURL = strings.TrimSpace(targetURL)

	// Build query for single URL's most recent valid capture
	// limit=-1: Return only the last result (most recent by timestamp)
	// filter=statuscode:[23]..: Match 2xx (success) or 3xx (redirect) status codes
	// Note: Using negative limit returns results from the end (most recent first)
	query := fmt.Sprintf(
		"url=%s&output=json&fl=original,timestamp,statuscode,mimetype&limit=-1&filter=statuscode:[23]..",
		url.QueryEscape(targetURL),
	)

	return query
}

// GetCDXRecordCount queries the CDX API to estimate the total number of records for a domain
// Uses showNumPages to get page count, then estimates records based on ~3000 records per page
// (CDX pages typically contain up to 3000 records each based on zipnum block size)
// Note: Includes collapse=urlkey to match the actual fetch query behavior
func (c *WaybackClient) GetCDXRecordCount(domain string) (int, error) {
	// Clean domain
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Use showNumPages to get the number of pages
	// Note: Each page contains approximately 3000 records (zipnum block size)
	// Include collapse=urlkey to match the actual fetch query
	rawURL := fmt.Sprintf(
		"https://web.archive.org/cdx/search/cdx?url=*.%s&collapse=urlkey&showNumPages=true",
		domain,
	)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/plain, */*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("CDX API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	// Response should be just a number (the page count)
	pageCount, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0, fmt.Errorf("failed to parse page count: %q", strings.TrimSpace(string(body)))
	}

	// Estimate total records: ~3000 records per page (CDX zipnum block size)
	// This is an approximation; actual count may vary
	estimatedRecords := pageCount * 3000

	return estimatedRecords, nil
}

// FetchLatestCDX fetches only the most recent valid (HTTP 200) CDX record for a specific URL
// This is much more efficient than fetching all records when you only need the latest accessible capture
// Returns nil if no valid capture exists
func (c *WaybackClient) FetchLatestCDX(targetURL string) (*models.CDXRecord, error) {
	// Build raw URL string
	rawURL := "https://web.archive.org/cdx/search/cdx?" + BuildCDXQueryLatest(targetURL)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers emulating a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://web.archive.org/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CDX API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip-compressed responses
	var reader io.Reader = resp.Body
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if strings.Contains(contentEncoding, "gzip") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var rawRows [][]string
	if err := json.Unmarshal(body, &rawRows); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Need at least header + 1 data row
	if len(rawRows) < 2 {
		return nil, nil // No valid capture found
	}

	// Skip header row (index 0), get first data row
	row := rawRows[1]
	if len(row) < 4 {
		return nil, nil // Malformed response
	}

	record := &models.CDXRecord{
		URL:       row[0],
		Timestamp: row[1],
	}

	// Parse status code
	if row[2] != "" && row[2] != "-" {
		if code, err := strconv.Atoi(row[2]); err == nil {
			record.StatusCode = &code
		}
	}

	// Parse MIME type
	if row[3] != "" && row[3] != "-" {
		mimeType := row[3]
		record.MimeType = &mimeType
	}

	return record, nil
}

// FetchCDX fetches CDX records for a domain with pagination support
// Returns records, resume key for next page, and any error
func (c *WaybackClient) FetchCDX(domain string, resumeKey string) (*models.CDXResponse, error) {
	// Build raw URL string with literal asterisk - DO NOT use url.URL as it encodes the asterisk
	rawURL := "https://web.archive.org/cdx/search/cdx?" + BuildCDXQuery(domain, resumeKey)

	// Create request with raw URL string
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers emulating a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://web.archive.org/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CDX API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip-compressed responses
	// Use case-insensitive check and handle variations like "gzip", "x-gzip", etc.
	var reader io.Reader = resp.Body
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if strings.Contains(contentEncoding, "gzip") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response body
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	return c.parseCDXResponse(body, domain)
}

// parseCDXResponse parses the CDX JSON response
// Format: [[header], [record1], [record2], ..., [], [resumeKey]]
// Each record: [original, timestamp, statuscode, mimetype]
// Resume key is a single-element array at the end (if more pages exist)
// Note: There may be an empty array [] before the resume key
func (c *WaybackClient) parseCDXResponse(body []byte, domain string) (*models.CDXResponse, error) {
	var rawRows [][]string
	if err := json.Unmarshal(body, &rawRows); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	response := &models.CDXResponse{
		Records: make([]models.CDXRecord, 0),
		HasMore: false,
	}

	if len(rawRows) == 0 {
		return response, nil
	}

	// Check last element for resume key FIRST (single-element array)
	// Per API docs, resume key appears at end as ["resumeKeyValue"]
	lastRow := rawRows[len(rawRows)-1]
	if len(lastRow) == 1 {
		response.ResumeKey = lastRow[0]
		response.HasMore = true
		// Remove the resume key row from processing
		rawRows = rawRows[:len(rawRows)-1]
	}

	// Skip header row (index 0) - it contains field names
	for i := 1; i < len(rawRows); i++ {
		row := rawRows[i]

		// Skip empty rows (API sometimes includes [] before resume key)
		if len(row) == 0 {
			continue
		}

		// Skip malformed rows (need at least 4 fields)
		if len(row) < 4 {
			continue
		}

		record := models.CDXRecord{
			URL:       row[0],
			Domain:    domain,
			Timestamp: row[1],
		}

		// Parse status code (may be empty or "-")
		if row[2] != "" && row[2] != "-" {
			if code, err := strconv.Atoi(row[2]); err == nil {
				record.StatusCode = &code
			}
		}

		// Parse MIME type (may be empty or "-")
		if row[3] != "" && row[3] != "-" {
			mimeType := row[3]
			record.MimeType = &mimeType
		}

		response.Records = append(response.Records, record)
	}

	return response, nil
}

// FetchResult contains the results of a CDX fetch operation
type FetchResult struct {
	Records    []models.CDXRecord
	ResumeKey  string // Current resume key (empty if complete)
	IsComplete bool   // True if all pages fetched
	Error      error  // Non-nil if fetch failed (but records may still be partial)
}

// BatchCallback is called after each batch of records is fetched
// Parameters: batch records, current resume key, total count so far, page number
// Return false to stop fetching
type BatchCallback func(batch []models.CDXRecord, resumeKey string, totalCount, page int) bool

// FetchAllCDX fetches all CDX records for a domain, handling pagination
// Calls the progress callback with (current count, page number) after each page
// Returns early if cancelled via the cancel channel
// On rate limiting (503), returns partial results with nil error
func (c *WaybackClient) FetchAllCDX(domain string, progress func(count, page int), cancel <-chan struct{}) ([]models.CDXRecord, error) {
	result := c.FetchAllCDXWithResume(domain, "", progress, nil, cancel)
	return result.Records, result.Error
}

// FetchAllCDXWithResume fetches CDX records starting from a resume key
// The batchCallback is called after each batch is fetched, allowing immediate processing
// Returns FetchResult with records, current resume key, and completion status
func (c *WaybackClient) FetchAllCDXWithResume(domain string, startResumeKey string, progress func(count, page int), batchCallback BatchCallback, cancel <-chan struct{}) FetchResult {
	var allRecords []models.CDXRecord
	resumeKey := startResumeKey
	page := 0
	retryCount := 0
	maxRetries := 3

	for {
		// Check for cancellation
		select {
		case <-cancel:
			return FetchResult{
				Records:    allRecords,
				ResumeKey:  resumeKey,
				IsComplete: false,
				Error:      fmt.Errorf("cancelled"),
			}
		default:
		}

		page++
		resp, err := c.FetchCDX(domain, resumeKey)
		if err != nil {
			// Check if it's a rate limit (503/429), timeout, or server error
			errStr := err.Error()
			isRateLimit := strings.Contains(errStr, "503") || strings.Contains(errStr, "429")
			isTimeout := strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded")

			if isRateLimit || isTimeout {
				if retryCount < maxRetries {
					retryCount++
					// Exponential backoff: 10s, 20s, 40s
					backoff := time.Duration(10<<(retryCount-1)) * time.Second
					if c.logger != nil {
						if isRateLimit {
							c.logger.Warn("Rate limited, waiting", "backoff", backoff, "retry", retryCount, "maxRetries", maxRetries)
						} else {
							c.logger.Warn("Request timeout, retrying", "backoff", backoff, "retry", retryCount, "maxRetries", maxRetries)
						}
					}
					select {
					case <-cancel:
						return FetchResult{
							Records:    allRecords,
							ResumeKey:  resumeKey,
							IsComplete: false,
							Error:      fmt.Errorf("cancelled"),
						}
					case <-time.After(backoff):
					}
					page-- // Retry same page
					continue
				}
				// Max retries exceeded, return what we have with resume key for later retry
				if c.logger != nil {
					c.logger.Warn("Max retries exceeded, returning partial results", "retries", maxRetries, "records", len(allRecords))
				}
				return FetchResult{
					Records:    allRecords,
					ResumeKey:  resumeKey,
					IsComplete: false,
					Error:      fmt.Errorf("request failed after %d retries: %w", maxRetries, err),
				}
			}
			// Other errors (e.g., invalid URL, connection refused) - return partial results
			return FetchResult{
				Records:    allRecords,
				ResumeKey:  resumeKey,
				IsComplete: false,
				Error:      err,
			}
		}

		// Reset retry count on success
		retryCount = 0

		allRecords = append(allRecords, resp.Records...)

		// Call batch callback to allow immediate processing (e.g., DB insert)
		if batchCallback != nil {
			if !batchCallback(resp.Records, resp.ResumeKey, len(allRecords), page) {
				// Callback requested stop
				return FetchResult{
					Records:    allRecords,
					ResumeKey:  resp.ResumeKey,
					IsComplete: false,
					Error:      nil,
				}
			}
		}

		// Report progress
		if progress != nil {
			progress(len(allRecords), page)
		}

		if c.logger != nil {
			c.logger.Info("CDX page fetched", "page", page, "pageRecords", len(resp.Records), "totalRecords", len(allRecords), "hasMore", resp.HasMore)
		}

		// Check if there are more pages
		if !resp.HasMore || resp.ResumeKey == "" {
			return FetchResult{
				Records:    allRecords,
				ResumeKey:  "",
				IsComplete: true,
				Error:      nil,
			}
		}

		resumeKey = resp.ResumeKey

		// Note: Rate limiting is handled by the caller (e.g., TUI's doDelayedFetch)
		// This allows configurable delays rather than hardcoded ones
	}
}
