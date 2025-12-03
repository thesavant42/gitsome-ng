package api

import (
	"fmt"
	"net/url"
	"testing"
)

// TestBuildCDXQuery verifies the query string is built correctly
func TestBuildCDXQuery(t *testing.T) {
	tests := []struct {
		domain    string
		resumeKey string
		wantURL   string
	}{
		{
			domain:    "bfl.ai",
			resumeKey: "",
			wantURL:   "url=*.bfl.ai",
		},
		{
			domain:    "archive.org",
			resumeKey: "",
			wantURL:   "url=*.archive.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			query := BuildCDXQuery(tt.domain, tt.resumeKey)

			// Verify asterisk is NOT encoded (should be *, not %2A)
			if containsSubstring(query, "%2A") || containsSubstring(query, "%2a") {
				t.Errorf("BuildCDXQuery() asterisk is encoded: got %q", query[:40])
			}

			// Verify the query starts with the expected URL pattern
			if !containsSubstring(query, tt.wantURL) {
				t.Errorf("BuildCDXQuery() = %q, want to contain %q", query, tt.wantURL)
			}

			t.Logf("Query: %s", query[:60]+"...")
		})
	}
}

// TestURLConstruction verifies the URL is built without double-encoding
func TestURLConstruction(t *testing.T) {
	domain := "bfl.ai"

	// Build URL the same way FetchCDX does
	reqURL := &url.URL{
		Scheme:   "https",
		Host:     "web.archive.org",
		Path:     "/cdx/search/cdx",
		RawQuery: BuildCDXQuery(domain, ""),
	}

	urlStr := reqURL.String()

	// The asterisk should NOT be encoded as %2A
	if containsSubstring(urlStr, "%2A") || containsSubstring(urlStr, "%2a") {
		t.Errorf("URL has encoded asterisk: %s", urlStr)
	}

	// The asterisk SHOULD appear literally
	if !containsSubstring(urlStr, "url=*.bfl.ai") {
		t.Errorf("URL missing literal asterisk: %s", urlStr)
	}

	t.Logf("Generated URL: %s", urlStr)
}

// TestExtractRootDomain tests domain extraction
func TestExtractRootDomain(t *testing.T) {
	tests := []struct {
		input    string
		wantRoot string
		wantErr  bool
	}{
		{"bfl.ai", "bfl.ai", false},
		{"playground.bfl.ai", "bfl.ai", false},
		{"https://playground.bfl.ai/", "bfl.ai", false},
		{"https://www.example.com/path?query=1", "example.com", false},
		{"test.dev.pci.westcoast.acme.com", "acme.com", false},
		{"", "", true}, // empty input should error
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExtractRootDomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractRootDomain(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.wantRoot {
				t.Errorf("ExtractRootDomain(%q) = %q, want %q", tt.input, got, tt.wantRoot)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestFetchCDXIntegration is an integration test that actually calls the API
// Run with: go test -v -run TestFetchCDXIntegration ./internal/api/
func TestFetchCDXIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewWaybackClient(nil)
	resp, err := client.FetchCDX("bfl.ai", "")

	if err != nil {
		t.Fatalf("FetchCDX failed: %v", err)
	}

	fmt.Printf("Fetched %d records for bfl.ai\n", len(resp.Records))
	fmt.Printf("HasMore: %v, ResumeKey: %q\n", resp.HasMore, resp.ResumeKey)

	if len(resp.Records) == 0 {
		t.Error("Expected at least some records for bfl.ai")
	}

	// Print first few records
	for i, r := range resp.Records {
		if i >= 3 {
			break
		}
		fmt.Printf("  %d: %s (status: %v)\n", i, r.URL, r.StatusCode)
	}
}
