package db

import (
	"database/sql"
	"fmt"

	"github.com/thesavant42/gitsome-ng/internal/models"
)

// InitSubdomainSchema initializes the subdomain tables
func (db *DB) InitSubdomainSchema() error {
	if _, err := db.conn.Exec(createTargetDomainsTable); err != nil {
		return fmt.Errorf("failed to create target_domains schema: %w", err)
	}
	if _, err := db.conn.Exec(createSubdomainsTable); err != nil {
		return fmt.Errorf("failed to create subdomains schema: %w", err)
	}
	return nil
}

// =============================================================================
// Target Domain Operations
// =============================================================================

// InsertTargetDomain adds a new domain to track for subdomain enumeration
func (db *DB) InsertTargetDomain(domain string) error {
	_, err := db.conn.Exec(insertTargetDomain, domain)
	if err != nil {
		return fmt.Errorf("failed to insert target domain: %w", err)
	}
	return nil
}

// GetTargetDomains returns all target domains
func (db *DB) GetTargetDomains() ([]models.TargetDomain, error) {
	rows, err := db.conn.Query(selectTargetDomains)
	if err != nil {
		return nil, fmt.Errorf("failed to query target domains: %w", err)
	}
	defer rows.Close()

	return scanTargetDomains(rows)
}

// GetTargetDomainsWithCounts returns all target domains with subdomain counts
func (db *DB) GetTargetDomainsWithCounts() ([]models.TargetDomain, error) {
	rows, err := db.conn.Query(selectTargetDomainsWithCounts)
	if err != nil {
		return nil, fmt.Errorf("failed to query target domains with counts: %w", err)
	}
	defer rows.Close()

	var domains []models.TargetDomain
	for rows.Next() {
		var d models.TargetDomain
		var addedAt string
		if err := rows.Scan(&d.ID, &d.Domain, &d.VTEnumerated, &d.CrtshEnumerated, &addedAt, &d.SubdomainCount); err != nil {
			return nil, fmt.Errorf("failed to scan target domain: %w", err)
		}
		d.AddedAt, _ = parseTimestamp(addedAt)
		domains = append(domains, d)
	}
	return domains, nil
}

// GetTargetDomain returns a single target domain by name
func (db *DB) GetTargetDomain(domain string) (*models.TargetDomain, error) {
	var d models.TargetDomain
	var addedAt string

	err := db.conn.QueryRow(selectTargetDomain, domain).Scan(
		&d.ID, &d.Domain, &d.VTEnumerated, &d.CrtshEnumerated, &addedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get target domain: %w", err)
	}

	d.AddedAt, _ = parseTimestamp(addedAt)
	return &d, nil
}

// MarkVTEnumerated marks a domain as enumerated via VirusTotal
func (db *DB) MarkVTEnumerated(domain string) error {
	_, err := db.conn.Exec(updateTargetDomainVTEnumerated, domain)
	if err != nil {
		return fmt.Errorf("failed to mark VT enumerated: %w", err)
	}
	return nil
}

// MarkCrtshEnumerated marks a domain as enumerated via crt.sh
func (db *DB) MarkCrtshEnumerated(domain string) error {
	_, err := db.conn.Exec(updateTargetDomainCrtshEnumerated, domain)
	if err != nil {
		return fmt.Errorf("failed to mark crt.sh enumerated: %w", err)
	}
	return nil
}

// DeleteTargetDomain removes a target domain and all its subdomains
func (db *DB) DeleteTargetDomain(domain string) error {
	// Delete subdomains first (foreign key)
	if _, err := db.conn.Exec(deleteSubdomainsByDomain, domain); err != nil {
		return fmt.Errorf("failed to delete subdomains: %w", err)
	}
	// Delete the domain
	if _, err := db.conn.Exec(deleteTargetDomain, domain); err != nil {
		return fmt.Errorf("failed to delete target domain: %w", err)
	}
	return nil
}

// scanTargetDomains scans rows into TargetDomain structs
func scanTargetDomains(rows *sql.Rows) ([]models.TargetDomain, error) {
	var domains []models.TargetDomain
	for rows.Next() {
		var d models.TargetDomain
		var addedAt string
		if err := rows.Scan(&d.ID, &d.Domain, &d.VTEnumerated, &d.CrtshEnumerated, &addedAt); err != nil {
			return nil, fmt.Errorf("failed to scan target domain: %w", err)
		}
		d.AddedAt, _ = parseTimestamp(addedAt)
		domains = append(domains, d)
	}
	return domains, nil
}

// =============================================================================
// Subdomain Operations
// =============================================================================

// InsertSubdomains inserts multiple subdomains into the database
// Uses INSERT OR IGNORE for deduplication, returns count of new records inserted
func (db *DB) InsertSubdomains(subdomains []models.Subdomain) (int, error) {
	if len(subdomains) == 0 {
		return 0, nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	insertStmt, err := tx.Prepare(insertSubdomain)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer insertStmt.Close()

	updateStmt, err := tx.Prepare(updateSubdomainMerge)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer updateStmt.Close()

	inserted := 0
	for _, s := range subdomains {
		// Try to insert
		result, err := insertStmt.Exec(s.Domain, s.Subdomain, s.Source, s.CNAMEs, s.AltNames, s.CertExpired)
		if err != nil {
			// If insert fails (duplicate), try to update/merge
			_, updateErr := updateStmt.Exec(s.CNAMEs, s.CNAMEs, s.AltNames, s.AltNames, s.CertExpired, s.Subdomain)
			if updateErr != nil {
				continue // Skip on both insert and update failure
			}
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return inserted, nil
}

// GetSubdomains returns all subdomains for a domain
func (db *DB) GetSubdomains(domain string) ([]models.Subdomain, error) {
	rows, err := db.conn.Query(selectSubdomains, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query subdomains: %w", err)
	}
	defer rows.Close()

	return scanSubdomains(rows)
}

// GetSubdomainsFiltered returns subdomains with filtering and pagination
func (db *DB) GetSubdomainsFiltered(filter models.SubdomainFilter) ([]models.Subdomain, int, error) {
	// Build search pattern
	searchPattern := ""
	if filter.SearchText != "" {
		searchPattern = "%" + filter.SearchText + "%"
	}

	// Get total count first
	var total int
	err := db.conn.QueryRow(selectSubdomainCountFiltered,
		filter.Domain, filter.SearchText, searchPattern, filter.Source, filter.Source, filter.CDXIndexed, filter.CDXIndexed,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subdomains: %w", err)
	}

	// Get paginated records
	rows, err := db.conn.Query(selectSubdomainsFiltered,
		filter.Domain, filter.SearchText, searchPattern, filter.Source, filter.Source, filter.CDXIndexed, filter.CDXIndexed,
		filter.Limit, filter.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query subdomains: %w", err)
	}
	defer rows.Close()

	subdomains, err := scanSubdomains(rows)
	if err != nil {
		return nil, 0, err
	}

	return subdomains, total, nil
}

// GetAllSubdomainsForDomain returns all subdomains for a domain (for export)
func (db *DB) GetAllSubdomainsForDomain(domain string) ([]models.Subdomain, error) {
	rows, err := db.conn.Query(selectAllSubdomainsForDomain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query all subdomains: %w", err)
	}
	defer rows.Close()

	return scanSubdomains(rows)
}

// GetSubdomainCount returns the total number of subdomains for a domain
func (db *DB) GetSubdomainCount(domain string) (int, error) {
	var count int
	err := db.conn.QueryRow(selectSubdomainCount, domain).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count subdomains: %w", err)
	}
	return count, nil
}

// GetSubdomainStats returns statistics for a domain's subdomains
func (db *DB) GetSubdomainStats(domain string) (*models.SubdomainStats, error) {
	var stats models.SubdomainStats

	err := db.conn.QueryRow(selectSubdomainStats, domain).Scan(
		&stats.Total, &stats.VTCount, &stats.CrtshCount, &stats.ImportCount,
		&stats.CDXCount, &stats.ExpiredCount,
	)
	if err == sql.ErrNoRows {
		return &models.SubdomainStats{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subdomain stats: %w", err)
	}

	return &stats, nil
}

// MarkSubdomainCDXIndexed marks a subdomain as processed via Wayback CDX
func (db *DB) MarkSubdomainCDXIndexed(subdomain string) error {
	_, err := db.conn.Exec(updateSubdomainCDXIndexed, subdomain)
	if err != nil {
		return fmt.Errorf("failed to mark CDX indexed: %w", err)
	}
	return nil
}

// DeleteSubdomain removes a single subdomain by ID
func (db *DB) DeleteSubdomain(id int64) error {
	_, err := db.conn.Exec(deleteSubdomain, id)
	if err != nil {
		return fmt.Errorf("failed to delete subdomain: %w", err)
	}
	return nil
}

// DeleteSubdomainsByDomain removes all subdomains for a domain
func (db *DB) DeleteSubdomainsByDomain(domain string) error {
	_, err := db.conn.Exec(deleteSubdomainsByDomain, domain)
	if err != nil {
		return fmt.Errorf("failed to delete subdomains by domain: %w", err)
	}
	return nil
}

// scanSubdomains scans rows into Subdomain structs
func scanSubdomains(rows *sql.Rows) ([]models.Subdomain, error) {
	var subdomains []models.Subdomain
	for rows.Next() {
		var s models.Subdomain
		var discoveredAt string
		var cnames, altNames sql.NullString

		if err := rows.Scan(
			&s.ID, &s.Domain, &s.Subdomain, &s.Source, &cnames, &altNames,
			&s.CertExpired, &s.CDXIndexed, &discoveredAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan subdomain: %w", err)
		}

		s.CNAMEs = cnames.String
		s.AltNames = altNames.String
		s.DiscoveredAt, _ = parseTimestamp(discoveredAt)

		subdomains = append(subdomains, s)
	}

	return subdomains, nil
}

// =============================================================================
// Application Settings Operations
// =============================================================================

// Setting keys
const (
	SettingVirusTotalAPIKey = "virustotal_api_key"
)

// SetSetting saves a setting to the database
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(upsertSetting, key, value)
	if err != nil {
		return fmt.Errorf("failed to save setting: %w", err)
	}
	return nil
}

// GetSetting retrieves a setting from the database
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(selectSetting, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil // Not found, return empty string
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting: %w", err)
	}
	return value, nil
}

// DeleteSetting removes a setting from the database
func (db *DB) DeleteSetting(key string) error {
	_, err := db.conn.Exec(deleteSetting, key)
	if err != nil {
		return fmt.Errorf("failed to delete setting: %w", err)
	}
	return nil
}

// GetVirusTotalAPIKey retrieves the VirusTotal API key from settings
func (db *DB) GetVirusTotalAPIKey() (string, error) {
	return db.GetSetting(SettingVirusTotalAPIKey)
}

// SetVirusTotalAPIKey saves the VirusTotal API key to settings
func (db *DB) SetVirusTotalAPIKey(apiKey string) error {
	return db.SetSetting(SettingVirusTotalAPIKey, apiKey)
}

