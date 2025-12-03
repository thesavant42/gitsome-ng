package models

import "time"

// CDXRecord represents a Wayback Machine CDX record
type CDXRecord struct {
	ID         int64
	URL        string
	Domain     string
	Timestamp  string  // 14-digit format: YYYYMMDDhhmmss
	StatusCode *int    // nullable - some records don't have status
	MimeType   *string // nullable - some records don't have mime type
	Tags       string
	FetchedAt  time.Time
}

// CDXResponse represents the response from a CDX API fetch
type CDXResponse struct {
	Records   []CDXRecord
	ResumeKey string // For pagination
	HasMore   bool
}

// WaybackFilter holds filter criteria for querying wayback records
type WaybackFilter struct {
	Domain     string
	MimeType   string // Filter by MIME type (e.g., "text/html", "image/")
	SearchText string // Filter by URL substring
	Tags       string // Filter by tags (e.g., "#important" or "review")
	Limit      int
	Offset     int
}

// WaybackDomainStats represents statistics for a cached domain
type WaybackDomainStats struct {
	Domain      string
	RecordCount int
}
