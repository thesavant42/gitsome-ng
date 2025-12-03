package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/thesavant42/gitsome-ng/internal/models"
)

// InsertWaybackRecords inserts multiple CDX records into the database
// Uses INSERT OR IGNORE to skip duplicates (based on unique URL constraint)
// Returns the number of records actually inserted
func (db *DB) InsertWaybackRecords(records []models.CDXRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertWaybackRecord)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, r := range records {
		var statusCode interface{}
		if r.StatusCode != nil {
			statusCode = *r.StatusCode
		}

		var mimeType interface{}
		if r.MimeType != nil {
			mimeType = *r.MimeType
		}

		result, err := stmt.Exec(r.URL, r.Domain, r.Timestamp, statusCode, mimeType, r.Tags)
		if err != nil {
			// Skip errors for individual records (e.g., duplicates)
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

// GetWaybackRecords retrieves all wayback records for a domain
func (db *DB) GetWaybackRecords(domain string) ([]models.CDXRecord, error) {
	rows, err := db.conn.Query(selectWaybackRecords, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query wayback records: %w", err)
	}
	defer rows.Close()

	return scanWaybackRecords(rows)
}

// GetWaybackRecordsFiltered retrieves wayback records with filtering and pagination
func (db *DB) GetWaybackRecordsFiltered(filter models.WaybackFilter) ([]models.CDXRecord, int, error) {
	// Build LIKE patterns
	mimePattern := ""
	if filter.MimeType != "" {
		mimePattern = "%" + filter.MimeType + "%"
	}
	searchPattern := ""
	if filter.SearchText != "" {
		searchPattern = "%" + filter.SearchText + "%"
	}
	tagPattern := ""
	if filter.Tags != "" {
		tagPattern = "%" + filter.Tags + "%"
	}

	// Get total count first
	var total int
	err := db.conn.QueryRow(selectWaybackRecordCountFiltered,
		filter.Domain, filter.MimeType, mimePattern, filter.SearchText, searchPattern, filter.Tags, tagPattern,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count wayback records: %w", err)
	}

	// Get paginated records
	rows, err := db.conn.Query(selectWaybackRecordsByFilter,
		filter.Domain, filter.MimeType, mimePattern, filter.SearchText, searchPattern, filter.Tags, tagPattern,
		filter.Limit, filter.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query wayback records: %w", err)
	}
	defer rows.Close()

	records, err := scanWaybackRecords(rows)
	if err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetWaybackRecordCount returns the total number of records for a domain
func (db *DB) GetWaybackRecordCount(domain string) (int, error) {
	var count int
	err := db.conn.QueryRow(selectWaybackRecordCount, domain).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count wayback records: %w", err)
	}
	return count, nil
}

// GetWaybackDomains returns all domains with cached wayback records
func (db *DB) GetWaybackDomains() ([]models.WaybackDomainStats, error) {
	rows, err := db.conn.Query(selectWaybackDomains)
	if err != nil {
		return nil, fmt.Errorf("failed to query wayback domains: %w", err)
	}
	defer rows.Close()

	var domains []models.WaybackDomainStats
	for rows.Next() {
		var d models.WaybackDomainStats
		if err := rows.Scan(&d.Domain, &d.RecordCount); err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, d)
	}

	return domains, nil
}

// UpdateWaybackRecordTags updates the tags for a wayback record
func (db *DB) UpdateWaybackRecordTags(id int64, tags string) error {
	_, err := db.conn.Exec(updateWaybackRecordTags, tags, id)
	if err != nil {
		return fmt.Errorf("failed to update wayback record tags: %w", err)
	}
	return nil
}

// DeleteWaybackRecord deletes a single wayback record by ID
func (db *DB) DeleteWaybackRecord(id int64) error {
	_, err := db.conn.Exec(deleteWaybackRecord, id)
	if err != nil {
		return fmt.Errorf("failed to delete wayback record: %w", err)
	}
	return nil
}

// DeleteWaybackRecordsByDomain deletes all wayback records for a domain
func (db *DB) DeleteWaybackRecordsByDomain(domain string) error {
	_, err := db.conn.Exec(deleteWaybackRecordsByDomain, domain)
	if err != nil {
		return fmt.Errorf("failed to delete wayback records for domain: %w", err)
	}
	return nil
}

// scanWaybackRecords scans rows into CDXRecord structs
func scanWaybackRecords(rows *sql.Rows) ([]models.CDXRecord, error) {
	var records []models.CDXRecord
	for rows.Next() {
		var r models.CDXRecord
		var fetchedAt string
		var statusCode sql.NullInt64
		var mimeType, timestamp sql.NullString

		if err := rows.Scan(
			&r.ID, &r.URL, &r.Domain, &timestamp, &statusCode, &mimeType, &r.Tags, &fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan wayback record: %w", err)
		}

		if timestamp.Valid {
			r.Timestamp = timestamp.String
		}
		if statusCode.Valid {
			code := int(statusCode.Int64)
			r.StatusCode = &code
		}
		if mimeType.Valid {
			mt := mimeType.String
			r.MimeType = &mt
		}
		r.FetchedAt, _ = parseTimestamp(fetchedAt)

		records = append(records, r)
	}

	return records, nil
}

// WaybackRecordExists checks if a URL already exists in the database
func (db *DB) WaybackRecordExists(url string) (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM wayback_records WHERE url = ?", url).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check wayback record existence: %w", err)
	}
	return count > 0, nil
}

// GetWaybackResumeState returns the last domain and record count for resuming
// This can be used to resume an interrupted fetch
func (db *DB) GetWaybackResumeState(domain string) (int, time.Time, error) {
	var count int
	var lastFetched string

	err := db.conn.QueryRow(`
		SELECT COUNT(*), COALESCE(MAX(fetched_at), '') 
		FROM wayback_records 
		WHERE domain = ?
	`, domain).Scan(&count, &lastFetched)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get resume state: %w", err)
	}

	var fetchedAt time.Time
	if lastFetched != "" {
		fetchedAt, _ = parseTimestamp(lastFetched)
	}

	return count, fetchedAt, nil
}

// WaybackFetchState represents the state of a CDX fetch operation
type WaybackFetchState struct {
	Domain       string
	ResumeKey    string
	TotalFetched int
	IsComplete   bool
	LastError    string
	UpdatedAt    time.Time
}

// SaveWaybackFetchState saves or updates the fetch state for a domain
func (db *DB) SaveWaybackFetchState(domain, resumeKey string, totalFetched int, isComplete bool, lastError string) error {
	_, err := db.conn.Exec(upsertWaybackFetchState, domain, resumeKey, totalFetched, isComplete, lastError)
	if err != nil {
		return fmt.Errorf("failed to save wayback fetch state: %w", err)
	}
	return nil
}

// GetWaybackFetchState retrieves the fetch state for a domain
func (db *DB) GetWaybackFetchState(domain string) (*WaybackFetchState, error) {
	var state WaybackFetchState
	var resumeKey, lastError sql.NullString
	var updatedAt string

	err := db.conn.QueryRow(selectWaybackFetchState, domain).Scan(
		&state.Domain, &resumeKey, &state.TotalFetched, &state.IsComplete, &lastError, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get wayback fetch state: %w", err)
	}

	state.ResumeKey = resumeKey.String
	state.LastError = lastError.String
	state.UpdatedAt, _ = parseTimestamp(updatedAt)

	return &state, nil
}

// DeleteWaybackFetchState removes the fetch state for a domain
func (db *DB) DeleteWaybackFetchState(domain string) error {
	_, err := db.conn.Exec(deleteWaybackFetchState, domain)
	if err != nil {
		return fmt.Errorf("failed to delete wayback fetch state: %w", err)
	}
	return nil
}
