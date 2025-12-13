package models

import "time"

// Subdomain represents a discovered subdomain record
type Subdomain struct {
	ID           int64
	Domain       string    // Parent/root domain
	Subdomain    string    // Full hostname (e.g., "api.example.com")
	Source       string    // "virustotal", "crtsh", "import"
	CNAMEs       string    // Comma-separated CNAMEs
	AltNames     string    // Comma-separated alt names from certificate
	CertExpired  bool      // Certificate is expired
	CDXIndexed   bool      // Has been processed via Wayback CDX
	DiscoveredAt time.Time // When the subdomain was discovered
}

// TargetDomain represents a domain being tracked for subdomain enumeration
type TargetDomain struct {
	ID              int64
	Domain          string
	VTEnumerated    bool      // Has been enumerated via VirusTotal
	CrtshEnumerated bool      // Has been enumerated via crt.sh
	AddedAt         time.Time // When the domain was added
	SubdomainCount  int       // Count of discovered subdomains (populated by join query)
}

// SubdomainStats holds statistics for a domain's subdomains
type SubdomainStats struct {
	Total        int
	VTCount      int
	CrtshCount   int
	ImportCount  int
	CDXCount     int
	ExpiredCount int
}

// SubdomainFilter holds filter criteria for querying subdomains
type SubdomainFilter struct {
	Domain     string
	SearchText string // Filter by subdomain substring
	Source     string // Filter by source ("virustotal", "crtsh", "import", or "" for all)
	CDXIndexed int    // -1 = all, 0 = not indexed, 1 = indexed
	Limit      int
	Offset     int
}

// VirusTotalSubdomainResponse represents the VT API response for subdomains
type VirusTotalSubdomainResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
		Self string `json:"self"`
	} `json:"links"`
	Meta struct {
		Cursor string `json:"cursor"`
	} `json:"meta"`
}

// CrtshEntry represents a single entry from crt.sh JSON response
type CrtshEntry struct {
	IssuerCAID        int    `json:"issuer_ca_id"`
	IssuerName        string `json:"issuer_name"`
	CommonName        string `json:"common_name"`
	NameValue         string `json:"name_value"` // Contains the subdomain(s), newline-separated for SANs
	ID                int64  `json:"id"`
	EntryTimestamp    string `json:"entry_timestamp"`
	NotBefore         string `json:"not_before"`
	NotAfter          string `json:"not_after"`
	SerialNumber      string `json:"serial_number"`
	ResultCount       int    `json:"result_count"`
}

// CrtshResponse is a slice of CrtshEntry
type CrtshResponse []CrtshEntry

