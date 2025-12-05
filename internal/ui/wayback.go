package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
	"github.com/thesavant42/gitsome-ng/internal/models"
)

// WaybackModel is the TUI model for Wayback Machine CDX browsing
type WaybackModel struct {
	client    *api.WaybackClient
	logger    *log.Logger
	database  *db.DB
	layout    Layout
	table     table.Model
	textInput textinput.Model
	spinner   spinner.Model
	progress  progress.Model

	// State
	domain          string
	records         []models.CDXRecord
	filteredRecords []models.CDXRecord
	totalRecords    int

	// View mode
	viewMode       waybackViewMode
	inputMode      waybackInputMode
	filterText     string
	filterMimeType string
	filterTag      string // Filter by tag (hashtag)

	// Fetch state
	fetching          bool
	fetchProgress     int
	fetchPage         int
	fetchTotalRecords int    // Total records from /howmany API
	fetchResumeKey    string // Resume key for continuing fetch after record count
	cancelFetch       chan struct{}
	fetchCancelled    bool      // Track if cancel was requested to prevent double-close
	fetchStartTime    time.Time // When overall fetch started (for throughput)
	requestStartTime  time.Time // When current request started (for stopwatch)

	// Domain browser state
	cachedDomains []models.WaybackDomainStats
	domainCursor  int

	// Pagination
	page     int
	pageSize int

	// Detail view state
	detailRecord *models.CDXRecord // Currently viewed record in detail modal
	detailScroll int               // Scroll position in detail view

	// UI state
	err                    error
	statusMsg              string
	quitting               bool
	returnToMain           bool
	layoutInitialized      bool
	switchToDomainsBrowser bool // Flag to switch to domains view after loading

	// Rate limiting settings - delay in milliseconds between requests
	requestDelay    int    // Delay in ms between requests (0-2000)
	settingsCursor  int    // Cursor position in settings view
	settingsEditing bool   // True when editing the delay value
	settingsInput   string // Text input for delay value
}

// Default delay values (in milliseconds)
const (
	defaultDelay = 1000 // 1 second delay (balanced)
	minDelay     = 0    // No delay (fastest, may get rate limited)
	maxDelay     = 2000 // 2 second delay (slowest, most respectful)
)

// getRequestDelay returns the delay as a time.Duration
func getRequestDelay(delayMs int) time.Duration {
	if delayMs < 0 {
		delayMs = 0
	}
	if delayMs > maxDelay {
		delayMs = maxDelay
	}
	return time.Duration(delayMs) * time.Millisecond
}

type waybackViewMode int

const (
	waybackViewInput    waybackViewMode = iota // Domain input screen
	waybackViewFetching                        // Fetching CDX records
	waybackViewTable                           // Table view of records
	waybackViewFilter                          // Filter input overlay
	waybackViewDomains                         // Browse cached domains
	waybackViewDetail                          // Detail modal for selected record
	waybackViewSettings                        // Rate limit settings
)

type waybackInputMode int

const (
	waybackInputDomain waybackInputMode = iota // Entering domain
	waybackInputFilter                         // Entering filter text
	waybackInputMime                           // Entering MIME filter
	waybackInputTag                            // Entering tag
)

// Messages
type waybackFetchProgressMsg struct {
	count int
	page  int
}

type waybackFetchCompleteMsg struct {
	records []models.CDXRecord
	err     error
}

// waybackBatchMsg is sent after each batch is fetched and inserted
type waybackBatchMsg struct {
	batch     []models.CDXRecord
	resumeKey string
	total     int
	page      int
	inserted  int
	hasMore   bool
	err       error
}

// waybackDelayCompleteMsg is sent after the rate limit delay completes
type waybackDelayCompleteMsg struct {
	resumeKey string
}

type waybackRecordsLoadedMsg struct {
	records []models.CDXRecord
	total   int
	err     error
}

// waybackRecordCountMsg is sent after querying the total record count from the CDX API's /howmany endpoint
type waybackRecordCountMsg struct {
	total int // Actual total record count from /howmany API
	err   error
}

type waybackDomainsLoadedMsg struct {
	domains []models.WaybackDomainStats
	err     error
}

// NewWaybackModel creates a new Wayback Machine TUI
func NewWaybackModel(logger *log.Logger, database *db.DB) WaybackModel {
	ti := textinput.New()
	ti.Placeholder = "Enter domain (e.g., playground.bfl.ai)"
	ti.Focus()
	ti.CharLimit = 200
	// Use default textinput styles - Charm component requires lipgloss.Style

	layout := DefaultLayout()

	columns := calculateWaybackColumns(layout.TableWidth)
	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)
	ApplyTableStyles(&t)

	spinnerModel := NewAppSpinner()
	prog := progress.New(
		progress.WithGradient("#FFFFFF", "#FF0000"), // White to red gradient (unscaled)
		progress.WithColorProfile(termenv.TrueColor),
	)
	prog.EmptyColor = "241" // Gray for unfilled portion (ANSI 256)

	return WaybackModel{
		client:       api.NewWaybackClient(nil), // Pass nil to silence logger during TUI
		logger:       logger,
		database:     database,
		layout:       layout,
		table:        t,
		textInput:    ti,
		spinner:      spinnerModel,
		progress:     prog,
		viewMode:     waybackViewInput,
		inputMode:    waybackInputDomain,
		page:         1,
		pageSize:     100,
		requestDelay: defaultDelay,
	}
}

// Init implements tea.Model
func (m WaybackModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick, m.loadCachedDomains())
}

// Update implements tea.Model
func (m WaybackModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.table.SetHeight(m.layout.TableHeight)
		m.textInput.Width = m.layout.InnerWidth - 10

		columns := calculateWaybackColumns(m.layout.TableWidth)
		m.table.SetColumns(columns)
		if len(m.filteredRecords) > 0 {
			m.updateTable()
		}
		m.layoutInitialized = true
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case waybackRecordCountMsg:
		// Got the total record count from the /howmany API
		if msg.err == nil && msg.total > 0 {
			m.fetchTotalRecords = msg.total
			if m.fetchResumeKey != "" {
				m.statusMsg = fmt.Sprintf("Resuming fetch (~%d records)...", msg.total)
			} else {
				m.statusMsg = fmt.Sprintf("Starting fetch (~%d records)...", msg.total)
			}
		} else if msg.err != nil {
			// Record count failed - show message but continue fetching
			m.statusMsg = fmt.Sprintf("Could not get total (will fetch all): %v", msg.err)
		}

		// Initialize progress bar and start timers
		initCmd := m.progress.SetPercent(0.0)
		m.fetchStartTime = time.Now()
		m.requestStartTime = time.Now()

		// Now start the actual fetch (resume if we have a key, fresh otherwise)
		if m.fetchResumeKey != "" {
			resumeKey := m.fetchResumeKey
			m.fetchResumeKey = "" // Clear it
			return m, tea.Batch(initCmd, m.doFetchWithResume(resumeKey))
		}
		return m, tea.Batch(initCmd, m.doFetch())

	case waybackBatchMsg:
		// Update progress from this batch
		m.fetchProgress = msg.total
		m.fetchPage = msg.page

		// Update status - keep it minimal since main display shows details
		if msg.inserted > 0 {
			m.statusMsg = fmt.Sprintf("Batch complete: +%d new records", msg.inserted)
		} else {
			m.statusMsg = "Batch complete: no new records (duplicates skipped)"
		}

		// Simple record-based progress: records_fetched / total_records
		var progressCmd tea.Cmd
		if m.fetchTotalRecords > 0 {
			percent := float64(m.fetchProgress) / float64(m.fetchTotalRecords)
			progressCmd = m.progress.SetPercent(percent)
		} else {
			// No total - use asymptotic approach like user enumeration
			divisor := float64(uint(1) << uint(msg.page))
			percent := 1.0 - (1.0 / divisor)
			progressCmd = m.progress.SetPercent(percent)
		}

		// Handle errors
		if msg.err != nil {
			m.fetching = false
			errStr := msg.err.Error()
			if errStr == "cancelled" {
				m.statusMsg = fmt.Sprintf("Fetch cancelled. Got %d records.", msg.total)
			} else {
				// Clean up error message - extract just the status code if it's an HTTP error
				cleanErr := errStr
				if strings.Contains(errStr, "status 504") {
					cleanErr = "Gateway timeout (504) - Wayback Machine busy"
				} else if strings.Contains(errStr, "status 503") {
					cleanErr = "Service unavailable (503) - Wayback Machine busy"
				} else if strings.Contains(errStr, "status 429") {
					cleanErr = "Rate limited (429) - too many requests"
				} else if strings.Contains(errStr, "<html>") || strings.Contains(errStr, "<body>") {
					// Strip HTML from error messages
					if idx := strings.Index(errStr, ":"); idx > 0 {
						cleanErr = strings.TrimSpace(errStr[:idx])
					}
				}
				// Don't set m.err for transient API errors - just show status message
				m.statusMsg = fmt.Sprintf("Stopped: %s. Got %d records.", cleanErr, msg.total)
			}
			return m, m.loadRecordsFromDB()
		}

		// If more pages, fetch next batch with rate limiting delay
		if msg.hasMore && msg.resumeKey != "" {
			delay := getRequestDelay(m.requestDelay)
			if delay > 0 {
				// Show delay status - simpler message since main display shows progress
				m.statusMsg = fmt.Sprintf("Rate limit: waiting %v...", delay)
				// Start delay, then fetch
				if progressCmd != nil {
					return m, tea.Batch(progressCmd, m.doDelayedFetch(msg.resumeKey, delay))
				}
				return m, m.doDelayedFetch(msg.resumeKey, delay)
			}
			// No delay - fetch immediately
			m.statusMsg = "Fetching next batch..."
			if progressCmd != nil {
				return m, tea.Batch(progressCmd, m.doFetchWithResume(msg.resumeKey))
			}
			return m, m.doFetchWithResume(msg.resumeKey)
		}

		// All done! Set progress to 100%
		m.fetching = false
		m.statusMsg = fmt.Sprintf("Fetch complete: %d records saved to database", msg.total)
		return m, tea.Batch(m.progress.SetPercent(1.0), m.loadRecordsFromDB())

	case waybackDelayCompleteMsg:
		// Delay is complete, reset stopwatch and fetch the next batch
		m.requestStartTime = time.Now()
		return m, m.doFetchWithResume(msg.resumeKey)

	case waybackFetchProgressMsg:
		m.fetchProgress = msg.count
		m.fetchPage = msg.page
		return m, nil

	case waybackFetchCompleteMsg:
		m.fetching = false
		if msg.err != nil {
			if msg.err.Error() == "cancelled" {
				m.statusMsg = fmt.Sprintf("Fetch cancelled. Got %d records (already saved to DB).", len(msg.records))
			} else {
				m.err = msg.err
				m.statusMsg = fmt.Sprintf("Error: %v (partial results saved to DB)", msg.err)
			}
		} else {
			m.statusMsg = fmt.Sprintf("Fetch complete: %d records saved to database", len(msg.records))
		}
		// Records are already inserted by batch callback, just load from DB
		return m, m.loadRecordsFromDB()

	case waybackRecordsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.records = msg.records
		m.totalRecords = msg.total
		m.applyFilters()
		m.updateTable()
		m.viewMode = waybackViewTable
		return m, nil

	case waybackDomainsLoadedMsg:
		if msg.err != nil {
			// Don't show error for initial load - just means no cached domains yet
			if m.viewMode != waybackViewInput {
				m.err = msg.err
			}
			return m, nil
		}
		m.cachedDomains = msg.domains
		m.domainCursor = 0
		// Only switch to domains view if user explicitly requested it (Tab key)
		// If we're on input view during init, just store the domains for display
		if m.switchToDomainsBrowser {
			m.viewMode = waybackViewDomains
			m.switchToDomainsBrowser = false
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

func (m WaybackModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.viewMode {
	case waybackViewInput:
		return m.handleInputKeys(msg)
	case waybackViewFetching:
		return m.handleFetchingKeys(msg)
	case waybackViewTable:
		return m.handleTableKeys(msg)
	case waybackViewFilter:
		return m.handleFilterKeys(msg)
	case waybackViewDomains:
		return m.handleDomainsKeys(msg)
	case waybackViewDetail:
		return m.handleDetailKeys(msg)
	case waybackViewSettings:
		return m.handleSettingsKeys(msg)
	default:
		return m, nil
	}
}

func (m WaybackModel) handleDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "v":
		// Close detail modal, return to table
		m.detailRecord = nil
		m.viewMode = waybackViewTable
		return m, nil

	case "up", "k":
		// Scroll up in detail view
		if m.detailScroll > 0 {
			m.detailScroll--
		}
		return m, nil

	case "down", "j":
		// Scroll down in detail view
		m.detailScroll++
		return m, nil

	case "enter":
		// Open live URL
		if m.detailRecord != nil {
			openURL(m.detailRecord.URL)
			m.statusMsg = "Opened live URL"
		}
		return m, nil

	case "a":
		// Open archived URL
		if m.detailRecord != nil {
			archiveURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", m.detailRecord.Timestamp, m.detailRecord.URL)
			openURL(archiveURL)
			m.statusMsg = "Opened archived URL"
		}
		return m, nil
	}
	return m, nil
}

func (m WaybackModel) handleInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.quitting = true
		m.returnToMain = true
		return m, tea.Quit

	case "enter":
		if m.textInput.Value() != "" {
			// Parse domain
			domain, err := api.ExtractRootDomain(m.textInput.Value())
			if err != nil {
				m.err = fmt.Errorf("invalid domain: %w", err)
				return m, nil
			}
			m.domain = domain
			m.err = nil

			// Check if we have cached records or in-progress fetch
			if m.database != nil {
				fetchState, _ := m.database.GetWaybackFetchState(domain)
				if fetchState != nil {
					if fetchState.IsComplete {
						// Load complete cached records
						m.statusMsg = fmt.Sprintf("Loading %d cached records for %s", fetchState.TotalFetched, domain)
						return m, m.loadRecordsFromDB()
					} else if fetchState.ResumeKey != "" {
						// Resume incomplete fetch - get record count first for progress
						m.statusMsg = fmt.Sprintf("Resuming from %d records...", fetchState.TotalFetched)
						m.viewMode = waybackViewFetching
						m.fetching = true
						m.fetchProgress = fetchState.TotalFetched
						m.fetchPage = fetchState.TotalFetched / 1000 // Approximate request count (1000 per batch)
						m.fetchTotalRecords = 0
						m.fetchCancelled = false
						m.cancelFetch = make(chan struct{})
						m.fetchStartTime = time.Now()
						m.requestStartTime = time.Now()
						m.fetchResumeKey = fetchState.ResumeKey // Store for after record count returns
						return m, tea.Batch(m.progress.SetPercent(0.0), m.getRecordCount())
					}
				}
			}

			// Start fresh fetch - first get record count for progress
			m.viewMode = waybackViewFetching
			m.fetching = true
			m.fetchProgress = 0
			m.fetchPage = 0
			m.fetchTotalRecords = 0
			m.fetchCancelled = false
			m.cancelFetch = make(chan struct{})
			m.statusMsg = "Querying total records..."
			m.fetchStartTime = time.Now()
			m.requestStartTime = time.Now()
			// Get record count first, then start fetching
			return m, tea.Batch(m.progress.SetPercent(0.0), m.getRecordCount())
		}
		return m, nil

	case "tab":
		// Switch to domain browser
		m.switchToDomainsBrowser = true
		return m, m.loadCachedDomains()

	case "ctrl+s":
		// Open settings (use ctrl+s so 's' can be typed in domain)
		m.viewMode = waybackViewSettings
		m.settingsCursor = 0
		m.settingsEditing = false
		m.settingsInput = fmt.Sprintf("%d", m.requestDelay)
		return m, nil

	default:
		// Pass to text input for normal typing
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m WaybackModel) handleFetchingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// Cancel fetch and return to main
		if !m.fetchCancelled && m.cancelFetch != nil {
			close(m.cancelFetch)
			m.fetchCancelled = true
		}
		m.quitting = true
		m.returnToMain = true
		return m, tea.Quit
	}
	return m, nil
}

func (m WaybackModel) handleSettingsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsEditing {
		// Editing mode - handle text input
		switch msg.String() {
		case "esc":
			// Cancel editing
			m.settingsEditing = false
			m.settingsInput = fmt.Sprintf("%d", m.requestDelay)
			return m, nil

		case "enter":
			// Save the value
			val := 0
			fmt.Sscanf(m.settingsInput, "%d", &val)
			if val < minDelay {
				val = minDelay
			}
			if val > maxDelay {
				val = maxDelay
			}
			m.requestDelay = val
			m.settingsEditing = false
			m.statusMsg = fmt.Sprintf("Delay set to %dms between requests", m.requestDelay)
			return m, nil

		case "backspace":
			if len(m.settingsInput) > 0 {
				m.settingsInput = m.settingsInput[:len(m.settingsInput)-1]
			}
			return m, nil

		default:
			// Only allow digits
			if len(msg.String()) == 1 {
				char := msg.String()[0]
				if char >= '0' && char <= '9' && len(m.settingsInput) < 3 {
					m.settingsInput += msg.String()
				}
			}
			return m, nil
		}
	}

	// Navigation mode
	switch msg.String() {
	case "esc", "q":
		// Return to input view
		m.viewMode = waybackViewInput
		return m, textinput.Blink

	case "enter", "e":
		// Start editing
		m.settingsEditing = true
		m.settingsInput = fmt.Sprintf("%d", m.requestDelay)
		return m, nil

	case "left", "h":
		// Decrease delay by 100ms
		m.requestDelay -= 100
		if m.requestDelay < minDelay {
			m.requestDelay = minDelay
		}
		return m, nil

	case "right", "l":
		// Increase delay by 100ms
		m.requestDelay += 100
		if m.requestDelay > maxDelay {
			m.requestDelay = maxDelay
		}
		return m, nil

	case "down", "j":
		// Decrease delay by 50ms
		m.requestDelay -= 50
		if m.requestDelay < minDelay {
			m.requestDelay = minDelay
		}
		return m, nil

	case "up", "k":
		// Increase delay by 50ms
		m.requestDelay += 50
		if m.requestDelay > maxDelay {
			m.requestDelay = maxDelay
		}
		return m, nil
	}
	return m, nil
}

func (m WaybackModel) handleTableKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Go back to input
		m.viewMode = waybackViewInput
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "up", "k":
		m.table.MoveUp(1)
		return m, nil

	case "down", "j":
		m.table.MoveDown(1)
		return m, nil

	case "enter":
		// Open live URL in browser
		if len(m.filteredRecords) > 0 {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filteredRecords) {
				url := m.filteredRecords[cursor].URL
				openURL(url)
				m.statusMsg = "Opened live URL"
			}
		}
		return m, nil

	case "a":
		// Open archived URL in browser
		if len(m.filteredRecords) > 0 {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filteredRecords) {
				record := m.filteredRecords[cursor]
				archiveURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", record.Timestamp, record.URL)
				openURL(archiveURL)
				m.statusMsg = "Opened archived URL"
			}
		}
		return m, nil

	case "v":
		// Show verbose detail modal
		if len(m.filteredRecords) > 0 {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filteredRecords) {
				record := m.filteredRecords[cursor]
				m.detailRecord = &record
				m.detailScroll = 0
				m.viewMode = waybackViewDetail
			}
		}
		return m, nil

	case "/":
		// Enter filter mode
		m.viewMode = waybackViewFilter
		m.inputMode = waybackInputFilter
		m.textInput.SetValue(m.filterText)
		m.textInput.Placeholder = "Filter by URL..."
		m.textInput.Focus()
		return m, textinput.Blink

	case "m":
		// Enter MIME filter mode
		m.viewMode = waybackViewFilter
		m.inputMode = waybackInputMime
		m.textInput.SetValue(m.filterMimeType)
		m.textInput.Placeholder = "Filter by MIME type (e.g., text/html, image/)..."
		m.textInput.Focus()
		return m, textinput.Blink

	case "t":
		// Add tag to selected record
		if len(m.filteredRecords) > 0 && m.database != nil {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filteredRecords) {
				// Enter tag input mode
				m.viewMode = waybackViewFilter
				m.inputMode = waybackInputTag
				record := m.filteredRecords[cursor]
				m.textInput.SetValue(record.Tags)
				m.textInput.Placeholder = "Enter tags (space-separated, #optional)..."
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}
		return m, nil

	case "#":
		// Enter tag filter mode
		m.viewMode = waybackViewFilter
		m.inputMode = waybackInputFilter // Reuse filter mode but pre-populate with #
		m.textInput.SetValue(m.filterTag)
		m.textInput.Placeholder = "Filter by tag (e.g., #important #review)..."
		m.textInput.Focus()
		return m, textinput.Blink

	case "c":
		// Clear filters and reload
		m.filterText = ""
		m.filterMimeType = ""
		m.filterTag = ""
		m.page = 1
		m.statusMsg = "Filters cleared"
		return m, m.loadRecordsFromDB()

	case "r":
		// Refresh from API - get record count first for progress
		m.viewMode = waybackViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchPage = 0
		m.fetchTotalRecords = 0
		m.fetchCancelled = false
		m.cancelFetch = make(chan struct{})
		m.statusMsg = "Querying total records..."
		m.fetchStartTime = time.Now()
		m.requestStartTime = time.Now()
		return m, tea.Batch(m.progress.SetPercent(0.0), m.getRecordCount())

	case "d":
		// Delete selected record
		if len(m.filteredRecords) > 0 && m.database != nil {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filteredRecords) {
				record := m.filteredRecords[cursor]
				if err := m.database.DeleteWaybackRecord(record.ID); err != nil {
					m.statusMsg = fmt.Sprintf("Delete error: %v", err)
				} else {
					m.statusMsg = "Record deleted"
					return m, m.loadRecordsFromDB()
				}
			}
		}
		return m, nil

	case "D":
		// Delete all records for domain AND fetch state (allows fresh restart)
		if m.database != nil && m.domain != "" {
			if err := m.database.DeleteWaybackRecordsByDomain(m.domain); err != nil {
				m.statusMsg = fmt.Sprintf("Delete error: %v", err)
			} else {
				// Also delete fetch state so user can start fresh
				_ = m.database.DeleteWaybackFetchState(m.domain)
				m.statusMsg = fmt.Sprintf("Deleted all records and fetch state for %s", m.domain)
				m.records = nil
				m.filteredRecords = nil
				m.totalRecords = 0
				m.viewMode = waybackViewInput
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}
		return m, nil

	case "X":
		// Delete all currently filtered/visible records (mass delete)
		if m.database != nil && len(m.filteredRecords) > 0 {
			deletedCount := 0
			for _, r := range m.filteredRecords {
				if err := m.database.DeleteWaybackRecord(r.ID); err == nil {
					deletedCount++
				}
			}
			m.statusMsg = fmt.Sprintf("Deleted %d filtered records", deletedCount)
			return m, m.loadRecordsFromDB()
		}
		return m, nil

	case "e":
		// Export to markdown
		if len(m.filteredRecords) > 0 {
			filename := fmt.Sprintf("wayback-%s-%s.md", m.domain, time.Now().Format("20060102-150405"))
			if err := exportWaybackToMarkdown(m.filteredRecords, m.domain, filename); err != nil {
				m.statusMsg = fmt.Sprintf("Export error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("Exported to %s", filename)
			}
		}
		return m, nil

	case "n", "right":
		// Next page
		maxPage := (m.totalRecords + m.pageSize - 1) / m.pageSize
		if m.page < maxPage {
			m.page++
			return m, m.loadRecordsFromDB()
		}

	case "p", "left":
		// Previous page
		if m.page > 1 {
			m.page--
			return m, m.loadRecordsFromDB()
		}
	}
	return m, nil
}

func (m WaybackModel) handleFilterKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Save the filter value based on input mode
		switch m.inputMode {
		case waybackInputFilter:
			input := m.textInput.Value()
			// Check if it's a tag filter (starts with # or used from # key)
			if strings.HasPrefix(strings.TrimSpace(input), "#") {
				m.filterTag = input
				m.filterText = "" // Clear URL filter
			} else {
				m.filterText = input
			}
		case waybackInputMime:
			m.filterMimeType = m.textInput.Value()
		case waybackInputTag:
			// Save tags to the selected record
			if len(m.filteredRecords) > 0 && m.database != nil {
				cursor := m.table.Cursor()
				if cursor >= 0 && cursor < len(m.filteredRecords) {
					record := m.filteredRecords[cursor]
					tags := m.textInput.Value()
					if err := m.database.UpdateWaybackRecordTags(record.ID, tags); err != nil {
						m.statusMsg = fmt.Sprintf("Error updating tags: %v", err)
					} else {
						m.statusMsg = "Tags updated"
					}
				}
			}
		}
		// Reset to page 1 when filter changes
		m.page = 1
		m.viewMode = waybackViewTable
		m.textInput.Placeholder = "Enter domain (e.g., playground.bfl.ai)"
		// Reload from database WITH the new filters applied
		return m, m.loadRecordsFromDB()

	case "esc":
		// Cancel filter editing without applying
		m.viewMode = waybackViewTable
		m.textInput.Placeholder = "Enter domain (e.g., playground.bfl.ai)"
		return m, nil

	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m WaybackModel) handleDomainsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.viewMode = waybackViewInput
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "up", "k":
		if m.domainCursor > 0 {
			m.domainCursor--
		}
		return m, nil

	case "down", "j":
		if m.domainCursor < len(m.cachedDomains)-1 {
			m.domainCursor++
		}
		return m, nil

	case "enter":
		if len(m.cachedDomains) > 0 && m.domainCursor < len(m.cachedDomains) {
			m.domain = m.cachedDomains[m.domainCursor].Domain
			return m, m.loadRecordsFromDB()
		}
	}
	return m, nil
}

// View implements tea.Model
func (m WaybackModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Wayback Machine CDX Browser"))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	switch m.viewMode {
	case waybackViewInput:
		contentBuilder.WriteString(m.renderInputView())
	case waybackViewFetching:
		contentBuilder.WriteString(m.renderFetchingView())
		// Include status message inside the main box during fetching
		if m.statusMsg != "" {
			contentBuilder.WriteString("\n")
			contentBuilder.WriteString(NormalStyle.Render(" " + m.statusMsg))
		}
	case waybackViewTable:
		contentBuilder.WriteString(m.renderTableView())
	case waybackViewFilter:
		contentBuilder.WriteString(m.renderFilterView())
	case waybackViewDomains:
		contentBuilder.WriteString(m.renderDomainsView())
	case waybackViewDetail:
		contentBuilder.WriteString(m.renderDetailView())
	case waybackViewSettings:
		contentBuilder.WriteString(m.renderSettingsView())
	}

	// Get the content string
	content := contentBuilder.String()

	// Calculate available height for main content box
	// Subtract: footer box (3 lines: 1 content + 2 border) + spacing (1 line) + border overhead (2 lines)
	mainAvailableHeight := m.layout.ViewportHeight - 6
	if mainAvailableHeight < 10 {
		mainAvailableHeight = 10
	}

	// Pad content to fill available height
	contentLines := strings.Count(content, "\n")
	if contentLines < mainAvailableHeight {
		content += strings.Repeat("\n", mainAvailableHeight-contentLines)
	}

	// Build result with two-box layout
	var result strings.Builder

	// First box: Main content (red border)
	mainBordered := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(mainAvailableHeight).
		Render(content)
	result.WriteString(mainBordered)
	result.WriteString("\n") // Spacing between boxes

	// Error message (if any) - shown above footer box (only for errors, not status)
	if m.err != nil {
		result.WriteString(fmt.Sprintf(" Error: %v\n", m.err))
	}

	// Second box: Help text (white border, 1 row high)
	helpText := m.getHelpTextPlain()
	textWidth := len(helpText)
	padding := (m.layout.InnerWidth - textWidth) / 2
	if padding < 0 {
		padding = 0
	}
	var footerContent strings.Builder
	if padding > 0 {
		footerContent.WriteString(strings.Repeat(" ", padding))
	}
	footerContent.WriteString(HintStyle.Render(helpText))
	// Fill remaining space
	remaining := m.layout.InnerWidth - padding - textWidth
	if remaining > 0 {
		footerContent.WriteString(strings.Repeat(" ", remaining))
	}

	// Apply white border to footer
	footerBordered := NewBorderStyleWithColor(colorWhite).
		Width(m.layout.InnerWidth).
		Height(1).
		Render(footerContent.String())
	result.WriteString(footerBordered)

	return result.String()
}

func (m WaybackModel) renderInputView() string {
	var b strings.Builder
	b.WriteString(" Domain: ")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(" Enter a domain or URL to fetch CDX records from the Wayback Machine."))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(" Examples: playground.bfl.ai, https://example.com/path"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(" Press Tab to browse cached domains."))

	// Show recently cached domains if available
	if len(m.cachedDomains) > 0 {
		b.WriteString("\n\n")
		b.WriteString(NormalStyle.Render(" Recently cached:"))
		b.WriteString("\n")

		// Show up to 10 most recent domains
		maxShow := 10
		if len(m.cachedDomains) < maxShow {
			maxShow = len(m.cachedDomains)
		}
		for i := 0; i < maxShow; i++ {
			d := m.cachedDomains[i]
			line := fmt.Sprintf("   %s (%d records)", d.Domain, d.RecordCount)
			b.WriteString(NormalStyle.Render(line))
			b.WriteString("\n")
		}
		if len(m.cachedDomains) > maxShow {
			more := len(m.cachedDomains) - maxShow
			b.WriteString(NormalStyle.Render(fmt.Sprintf("   ... and %d more (press Tab to browse)", more)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m WaybackModel) renderFetchingView() string {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(AccentStyle.Render(fmt.Sprintf("Fetching CDX records for %s...", m.domain)))
	b.WriteString("\n\n")

	// Show progress with actual total if known (from /howmany endpoint)
	if m.fetchTotalRecords > 0 {
		percent := 0
		if m.fetchTotalRecords > 0 {
			percent = (m.fetchProgress * 100) / m.fetchTotalRecords
		}
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" Records: %d / %d", m.fetchProgress, m.fetchTotalRecords)))
		b.WriteString(DimStyle.Render(fmt.Sprintf("  (%d%%)", percent)))
	} else {
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" Records fetched: %d", m.fetchProgress)))
		b.WriteString(DimStyle.Render("  (calculating total...)"))
	}
	b.WriteString("\n")

	// API request number - starts at 1
	requestNum := m.fetchPage
	if requestNum < 1 {
		requestNum = 1
	}

	// Show request progress (estimate total requests as totalRecords / 1000)
	if m.fetchTotalRecords > 0 {
		estimatedTotalRequests := (m.fetchTotalRecords + 999) / 1000 // Round up
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" API request: #%d of ~%d", requestNum, estimatedTotalRequests)))
	} else {
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" API request: #%d", requestNum)))
	}
	b.WriteString("\n")

	// Stopwatch for current request
	if !m.requestStartTime.IsZero() {
		elapsed := time.Since(m.requestStartTime)
		b.WriteString(DimStyle.Render(fmt.Sprintf(" Request time: %.1fs", elapsed.Seconds())))
	}
	b.WriteString("\n\n")

	// Progress bar (indeterminate style - just show activity)
	progressBarWidth := m.layout.InnerWidth - 4
	if progressBarWidth < 40 {
		progressBarWidth = 40
	}
	m.progress.Width = progressBarWidth
	b.WriteString(" ")
	b.WriteString(m.progress.View())
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(" Esc: cancel fetch"))
	return b.String()
}

func (m WaybackModel) renderTableView() string {
	var b strings.Builder

	// Query info
	queryInfo := fmt.Sprintf(" Domain: %s", m.domain)
	if m.filterText != "" || m.filterMimeType != "" || m.filterTag != "" {
		queryInfo += "  |  Filters:"
		if m.filterText != "" {
			queryInfo += fmt.Sprintf(" URL~'%s'", m.filterText)
		}
		if m.filterMimeType != "" {
			queryInfo += fmt.Sprintf(" MIME~'%s'", m.filterMimeType)
		}
		if m.filterTag != "" {
			queryInfo += fmt.Sprintf(" TAG~'%s'", m.filterTag)
		}
	}
	maxPage := (m.totalRecords + m.pageSize - 1) / m.pageSize
	if maxPage < 1 {
		maxPage = 1
	}
	queryInfo += fmt.Sprintf("  |  Page %d/%d  |  Total: %d", m.page, maxPage, m.totalRecords)
	b.WriteString(AccentStyle.Render(queryInfo))
	b.WriteString("\n\n")

	// Info row 1: Position tracking
	currentRow := m.table.Cursor() + 1
	totalRows := len(m.filteredRecords)
	b.WriteString(fmt.Sprintf(" Row %d/%d", currentRow, totalRows))
	b.WriteString("\n")

	// Info row 2: Viewport information
	pageOffset := (m.page - 1) * m.pageSize
	b.WriteString(fmt.Sprintf(" Viewing records %d-%d of %d total", pageOffset+1, pageOffset+len(m.filteredRecords), m.totalRecords))
	b.WriteString("\n\n")

	// Table
	b.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

	return b.String()
}

func (m WaybackModel) renderFilterView() string {
	var b strings.Builder
	b.WriteString(m.renderTableView())
	b.WriteString("\n\n")
	b.WriteString(AccentStyle.Render(" Filter: "))
	b.WriteString(m.textInput.View())
	return b.String()
}

func (m WaybackModel) renderDomainsView() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(" Cached Domains"))
	b.WriteString("\n\n")

	if len(m.cachedDomains) == 0 {
		b.WriteString(DimStyle.Render(" No cached domains found. Enter a domain to fetch records."))
		return b.String()
	}

	selectedStyle := SelectedStyle.Width(m.layout.InnerWidth)
	normalStyle := NormalStyle.Width(m.layout.InnerWidth)

	for i, d := range m.cachedDomains {
		line := fmt.Sprintf("  %s (%d records)", d.Domain, d.RecordCount)
		if i == m.domainCursor {
			b.WriteString(selectedStyle.Render("> " + line))
		} else {
			b.WriteString(normalStyle.Render("  " + line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m WaybackModel) renderSettingsView() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(" Request Delay Settings"))
	b.WriteString("\n\n")

	// Description
	b.WriteString(NormalStyle.Render(" Configure the delay between API requests to Wayback Machine."))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render(" Higher values are slower but more respectful to the server."))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render(" Lower values are faster but may get rate limited."))
	b.WriteString("\n\n")

	// Draw slider bar (0ms = left/fast, 2000ms = right/slow)
	sliderWidth := 40
	filledWidth := ((maxDelay - m.requestDelay) * sliderWidth) / maxDelay
	if filledWidth > sliderWidth {
		filledWidth = sliderWidth
	}
	if filledWidth < 0 {
		filledWidth = 0
	}

	sliderBar := strings.Repeat("█", filledWidth) + strings.Repeat("░", sliderWidth-filledWidth)

	b.WriteString(NormalStyle.Render(" Request Speed:"))
	b.WriteString("\n\n")

	// Slider with labels
	b.WriteString(DimStyle.Render(" Fast "))
	b.WriteString(AccentStyle.Render(sliderBar))
	b.WriteString(DimStyle.Render(" Slow"))
	b.WriteString("\n\n")

	// Current value
	if m.settingsEditing {
		b.WriteString(NormalStyle.Render(" Delay: "))
		b.WriteString(AccentStyle.Render(m.settingsInput))
		b.WriteString(AccentStyle.Render("_"))
		b.WriteString(NormalStyle.Render(" ms"))
	} else {
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" Delay: %d ms", m.requestDelay)))
	}
	b.WriteString("\n")
	delaySeconds := float64(m.requestDelay) / 1000.0
	b.WriteString(DimStyle.Render(fmt.Sprintf(" (%.1f seconds between requests)", delaySeconds)))
	b.WriteString("\n\n")

	// Presets
	b.WriteString(DimStyle.Render(" Presets:"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("      0 ms = no delay (fastest, may be rate limited)"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("   1000 ms = 1 second delay (balanced, default)"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("   2000 ms = 2 second delay (slowest, safest)"))
	b.WriteString("\n\n")

	b.WriteString(HintStyle.Render(" ←/→: ±100ms | ↑/↓: ±50ms | e: type value | Esc: back"))

	return b.String()
}

func (m WaybackModel) getHelpText() string {
	switch m.viewMode {
	case waybackViewInput:
		return HintStyle.Render("Enter: fetch | Tab: browse cached | ^S: settings | Esc: back")
	case waybackViewFetching:
		return HintStyle.Render("Esc: cancel fetch")
	case waybackViewTable:
		return HintStyle.Render("Enter: open | a: archive | v: details | /: filter | m: MIME | #: tag filter | t: tag | c: clear | d: del | X: mass del | e: export | Esc: back")
	case waybackViewFilter:
		return HintStyle.Render("Enter: apply filter | Esc: cancel")
	case waybackViewDomains:
		return HintStyle.Render("Enter: select | up/down: navigate | Esc: back")
	case waybackViewDetail:
		return HintStyle.Render("Enter: open live | a: archive | j/k: scroll | Esc: close")
	case waybackViewSettings:
		if m.settingsEditing {
			return HintStyle.Render("Enter: save | Esc: cancel")
		}
		return HintStyle.Render("←/→: adjust ±10 | ↑/↓: adjust ±1 | e: edit | Esc: back")
	default:
		return ""
	}
}

// getHelpTextPlain returns the help text without styling (for two-box layout)
func (m WaybackModel) getHelpTextPlain() string {
	switch m.viewMode {
	case waybackViewInput:
		return "Enter: fetch | Tab: cached | ^S: settings | Esc: back"
	case waybackViewFetching:
		return "Esc: cancel fetch"
	case waybackViewTable:
		return "Enter: open | a: archive | v: view | /: filter | t: tag | Esc: back"
	case waybackViewFilter:
		return "Enter: apply filter | Esc: cancel"
	case waybackViewDomains:
		return "Enter: select | up/down: navigate | Esc: back"
	case waybackViewDetail:
		return "Enter: open live | a: archive | j/k: scroll | Esc: close"
	case waybackViewSettings:
		if m.settingsEditing {
			return "Enter: save | Esc: cancel"
		}
		return "←/→: ±10 | ↑/↓: ±1 | e: edit value | Esc: back"
	default:
		return ""
	}
}

func (m WaybackModel) renderDetailView() string {
	if m.detailRecord == nil {
		return DimStyle.Render("No record selected")
	}

	var b strings.Builder
	r := m.detailRecord

	// Title - use TitleStyle (red) not AccentStyle (yellow is for help/time-sensitive)
	b.WriteString(TitleStyle.Render(" URL Details"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.layout.InnerWidth-2))
	b.WriteString("\n\n")

	// Calculate available width for wrapped text (with padding)
	wrapWidth := m.layout.InnerWidth - 6
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	// URL (wrapped)
	b.WriteString(DimStyle.Render(" URL:"))
	b.WriteString("\n")
	wrappedURL := wrapText(r.URL, wrapWidth)
	lines := strings.Split(wrappedURL, "\n")

	// Apply scroll offset
	startLine := m.detailScroll
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if startLine < 0 {
		startLine = 0
	}

	// Show visible lines (limit to viewport)
	maxLines := m.layout.TableHeight - 10
	if maxLines < 5 {
		maxLines = 5
	}
	endLine := startLine + maxLines
	if endLine > len(lines) {
		endLine = len(lines)
	}

	for i := startLine; i < endLine; i++ {
		b.WriteString("   ")
		b.WriteString(NormalStyle.Render(lines[i]))
		b.WriteString("\n")
	}

	// Show scroll indicator if needed
	if len(lines) > maxLines {
		b.WriteString(DimStyle.Render(fmt.Sprintf("   [%d-%d of %d lines]", startLine+1, endLine, len(lines))))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Timestamp
	b.WriteString(DimStyle.Render(" Timestamp: "))
	if r.Timestamp != "" {
		formatted := formatWaybackTimestamp(r.Timestamp)
		b.WriteString(NormalStyle.Render(formatted))
		b.WriteString(DimStyle.Render(" (" + r.Timestamp + ")"))
	} else {
		b.WriteString(DimStyle.Render("N/A"))
	}
	b.WriteString("\n\n")

	// Status Code
	b.WriteString(DimStyle.Render(" Status: "))
	if r.StatusCode != nil {
		b.WriteString(NormalStyle.Render(fmt.Sprintf("%d", *r.StatusCode)))
	} else {
		b.WriteString(DimStyle.Render("N/A"))
	}
	b.WriteString("\n\n")

	// MIME Type
	b.WriteString(DimStyle.Render(" MIME Type: "))
	if r.MimeType != nil {
		b.WriteString(NormalStyle.Render(*r.MimeType))
	} else {
		b.WriteString(DimStyle.Render("N/A"))
	}
	b.WriteString("\n\n")

	// Tags
	b.WriteString(DimStyle.Render(" Tags: "))
	if r.Tags != "" {
		b.WriteString(NormalStyle.Render(r.Tags))
	} else {
		b.WriteString(DimStyle.Render("(none)"))
	}
	b.WriteString("\n\n")

	// Archive URL
	archiveURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", r.Timestamp, r.URL)
	b.WriteString(DimStyle.Render(" Archive URL:"))
	b.WriteString("\n")
	wrappedArchive := wrapText(archiveURL, wrapWidth)
	for _, line := range strings.Split(wrappedArchive, "\n") {
		b.WriteString("   ")
		b.WriteString(NormalStyle.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

// wrapText wraps text to fit within a given width
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= width {
			result.WriteString(remaining)
			break
		}

		// Find a good break point (prefer after / ? & = characters for URLs)
		breakPoint := width
		for i := width; i > width/2; i-- {
			c := remaining[i-1]
			if c == '/' || c == '?' || c == '&' || c == '=' || c == '-' || c == '_' {
				breakPoint = i
				break
			}
		}

		result.WriteString(remaining[:breakPoint])
		result.WriteString("\n")
		remaining = remaining[breakPoint:]
	}

	return result.String()
}

// formatWaybackTimestamp converts a 14-digit Wayback timestamp to human-readable format
// Input: YYYYMMDDhhmmss (e.g., "20231015143022")
// Output: "2023-10-15 14:30:22"
func formatWaybackTimestamp(ts string) string {
	if len(ts) < 14 {
		return ts // Return as-is if not standard format
	}
	return fmt.Sprintf("%s-%s-%s %s:%s:%s",
		ts[0:4],   // Year
		ts[4:6],   // Month
		ts[6:8],   // Day
		ts[8:10],  // Hour
		ts[10:12], // Minute
		ts[12:14], // Second
	)
}

// Commands

func (m WaybackModel) doFetch() tea.Cmd {
	return m.doFetchWithResume("")
}

func (m WaybackModel) doFetchWithResume(resumeKey string) tea.Cmd {
	return func() tea.Msg {
		// Check for cancellation before starting
		select {
		case <-m.cancelFetch:
			return waybackBatchMsg{err: fmt.Errorf("cancelled"), hasMore: false}
		default:
		}

		// Fetch ONE batch only
		resp, err := m.client.FetchCDX(m.domain, resumeKey)
		if err != nil {
			return waybackBatchMsg{err: err, hasMore: false}
		}

		// Insert this batch into DB
		inserted := 0
		if m.database != nil && len(resp.Records) > 0 {
			inserted, _ = m.database.InsertWaybackRecords(resp.Records)
			// Save fetch state after this batch
			_ = m.database.SaveWaybackFetchState(
				m.domain,
				resp.ResumeKey,
				m.fetchProgress+len(resp.Records),
				!resp.HasMore,
				"",
			)
		}

		return waybackBatchMsg{
			batch:     resp.Records,
			resumeKey: resp.ResumeKey,
			total:     m.fetchProgress + len(resp.Records),
			page:      m.fetchPage + 1,
			inserted:  inserted,
			hasMore:   resp.HasMore,
			err:       nil,
		}
	}
}

// doDelayedFetch waits for the specified delay before signaling to fetch the next batch
func (m WaybackModel) doDelayedFetch(resumeKey string, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		// Check for cancellation before sleeping
		select {
		case <-m.cancelFetch:
			return waybackBatchMsg{err: fmt.Errorf("cancelled"), hasMore: false}
		default:
		}

		// Sleep for the rate limit delay
		time.Sleep(delay)

		// Check for cancellation after sleeping
		select {
		case <-m.cancelFetch:
			return waybackBatchMsg{err: fmt.Errorf("cancelled"), hasMore: false}
		default:
		}

		return waybackDelayCompleteMsg{resumeKey: resumeKey}
	}
}

func (m WaybackModel) loadRecordsFromDB() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return waybackRecordsLoadedMsg{err: fmt.Errorf("no database")}
		}

		filter := models.WaybackFilter{
			Domain:     m.domain,
			MimeType:   m.filterMimeType,
			SearchText: m.filterText,
			Tags:       m.filterTag,
			Limit:      m.pageSize,
			Offset:     (m.page - 1) * m.pageSize,
		}

		records, total, err := m.database.GetWaybackRecordsFiltered(filter)
		return waybackRecordsLoadedMsg{records: records, total: total, err: err}
	}
}

func (m WaybackModel) loadCachedDomains() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return waybackDomainsLoadedMsg{err: fmt.Errorf("no database")}
		}
		domains, err := m.database.GetWaybackDomains()
		return waybackDomainsLoadedMsg{domains: domains, err: err}
	}
}

// getRecordCount queries the CDX API's /howmany endpoint for the total number of records
// This is used to show progress during fetch
func (m WaybackModel) getRecordCount() tea.Cmd {
	return func() tea.Msg {
		total, err := m.client.GetCDXRecordCount(m.domain)
		if err != nil {
			// Non-fatal - we can still fetch without knowing total
			return waybackRecordCountMsg{total: 0, err: err}
		}
		return waybackRecordCountMsg{total: total, err: nil}
	}
}

func (m *WaybackModel) applyFilters() {
	// Filters are applied in the database query now
	m.filteredRecords = m.records
}

func (m *WaybackModel) updateTable() {
	columns := calculateWaybackColumns(m.layout.TableWidth)

	urlW := columns[0].Width
	tsW := columns[1].Width
	statusW := columns[2].Width
	mimeW := columns[3].Width

	truncate := func(s string, w int) string {
		if len(s) <= w {
			return s
		}
		if w <= 3 {
			return s[:w]
		}
		return s[:w-3] + "..."
	}

	rows := make([]table.Row, len(m.filteredRecords))
	for i, r := range m.filteredRecords {
		// Format timestamp: YYYYMMDDhhmmss -> YYYY-MM-DD HH:MM
		ts := r.Timestamp
		if len(ts) >= 12 {
			ts = fmt.Sprintf("%s-%s-%s %s:%s", ts[0:4], ts[4:6], ts[6:8], ts[8:10], ts[10:12])
		}

		status := "-"
		if r.StatusCode != nil {
			status = fmt.Sprintf("%d", *r.StatusCode)
		}

		mime := "-"
		if r.MimeType != nil {
			mime = *r.MimeType
		}

		rows[i] = table.Row{
			truncate(r.URL, urlW),
			truncate(ts, tsW),
			truncate(status, statusW),
			truncate(mime, mimeW),
		}
	}

	m.table.SetColumns(columns)
	m.table.SetRows(rows)
	m.table.SetCursor(0) // Start at first visible row
}

func calculateWaybackColumns(totalW int) []table.Column {
	if totalW < 80 {
		totalW = 80
	}

	// Fixed widths for smaller columns - give them breathing room
	tsW := 18    // YYYY-MM-DD HH:MM:SS with padding
	statusW := 8 // Status code with padding
	mimeW := 22  // MIME type with padding

	fixedTotal := tsW + statusW + mimeW

	// URL gets remaining space (truncation is OK since we have detail view)
	urlW := totalW - fixedTotal
	if urlW < 30 {
		urlW = 30
	}

	// Verify exact match
	actualTotal := urlW + tsW + statusW + mimeW
	if actualTotal != totalW {
		urlW += (totalW - actualTotal)
	}

	return []table.Column{
		{Title: "URL", Width: urlW},
		{Title: "Timestamp", Width: tsW},
		{Title: "Status", Width: statusW},
		{Title: "MIME Type", Width: mimeW},
	}
}

// ShouldReturnToMain returns true if user wants to go back
func (m WaybackModel) ShouldReturnToMain() bool {
	return m.returnToMain
}

// Helper functions

func exportWaybackToMarkdown(records []models.CDXRecord, domain, filename string) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Wayback Machine CDX Records: %s\n\n", domain))
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Total Records: %d\n\n", len(records)))

	b.WriteString("## Records\n\n")
	b.WriteString("| URL | Timestamp | Status | MIME Type | Archive Link |\n")
	b.WriteString("|-----|-----------|--------|-----------|-------------|\n")

	for _, r := range records {
		ts := r.Timestamp
		if len(ts) >= 12 {
			ts = fmt.Sprintf("%s-%s-%s %s:%s", ts[0:4], ts[4:6], ts[6:8], ts[8:10], ts[10:12])
		}

		status := "-"
		if r.StatusCode != nil {
			status = fmt.Sprintf("%d", *r.StatusCode)
		}

		mime := "-"
		if r.MimeType != nil {
			mime = *r.MimeType
		}

		archiveURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", r.Timestamp, r.URL)

		// Escape pipes in URL
		escapedURL := strings.ReplaceAll(r.URL, "|", "\\|")

		b.WriteString(fmt.Sprintf("| [%s](%s) | %s | %s | %s | [Archive](%s) |\n",
			escapedURL, r.URL, ts, status, mime, archiveURL))
	}

	return writeStringToFile(filename, b.String())
}

func writeStringToFile(filename, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

// RunWaybackBrowser starts the Wayback Machine CDX browser TUI (starts at domain input)
func RunWaybackBrowser(logger *log.Logger, database *db.DB) error {
	model := NewWaybackModel(logger, database)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(WaybackModel)
	if !ok {
		return nil
	}

	if m.ShouldReturnToMain() {
		return nil
	}

	return nil
}

// RunWaybackCacheBrowser starts the Wayback Machine browser directly in cached domains view
func RunWaybackCacheBrowser(logger *log.Logger, database *db.DB) error {
	model := NewWaybackModel(logger, database)
	// Start in domain browser mode and trigger load
	model.switchToDomainsBrowser = true
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(WaybackModel)
	if !ok {
		return nil
	}

	if m.ShouldReturnToMain() {
		return nil
	}

	return nil
}
