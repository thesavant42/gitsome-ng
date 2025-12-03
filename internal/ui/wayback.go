package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/stopwatch"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
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
	stopwatch stopwatch.Model
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
	fetching       bool
	fetchProgress  int
	fetchPage      int
	fetchEstimated int // Estimated total records (for progress bar)
	cancelFetch    chan struct{}
	fetchCancelled bool // Track if cancel was requested to prevent double-close

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
	err               error
	statusMsg         string
	quitting          bool
	returnToMain      bool
	layoutInitialized bool
}

type waybackViewMode int

const (
	waybackViewInput    waybackViewMode = iota // Domain input screen
	waybackViewFetching                        // Fetching CDX records
	waybackViewTable                           // Table view of records
	waybackViewFilter                          // Filter input overlay
	waybackViewDomains                         // Browse cached domains
	waybackViewDetail                          // Detail modal for selected record
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

type waybackRecordsLoadedMsg struct {
	records []models.CDXRecord
	total   int
	err     error
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
	// Style text input to match app theme (white text, not yellow)
	ti.TextStyle = NormalStyle
	ti.PromptStyle = NormalStyle

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
	sw := stopwatch.NewWithInterval(100 * time.Millisecond)
	prog := progress.New(progress.WithDefaultGradient())

	return WaybackModel{
		client:    api.NewWaybackClient(nil), // Pass nil to silence logger during TUI
		logger:    logger,
		database:  database,
		layout:    layout,
		table:     t,
		textInput: ti,
		spinner:   spinnerModel,
		stopwatch: sw,
		progress:  prog,
		viewMode:  waybackViewInput,
		inputMode: waybackInputDomain,
		page:      1,
		pageSize:  100,
	}
}

// Init implements tea.Model
func (m WaybackModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
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

	case stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd

	case stopwatch.StartStopMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case waybackFetchProgressMsg:
		m.fetchProgress = msg.count
		m.fetchPage = msg.page
		// Estimate total based on first page results (batch size)
		// This is a rough estimate since we don't know actual total until complete
		if msg.page == 1 && msg.count > 0 {
			// If first page has results, assume there might be more
			// Estimate conservatively (don't over-promise)
			m.fetchEstimated = msg.count * 5 // Assume ~5 pages worth
		}
		return m, nil

	case waybackFetchCompleteMsg:
		m.fetching = false
		if msg.err != nil {
			if msg.err.Error() == "cancelled" {
				m.statusMsg = fmt.Sprintf("Fetch cancelled. Got %d records.", len(msg.records))
			} else {
				m.err = msg.err
				m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			}
		}
		// Save records to database
		if m.database != nil && len(msg.records) > 0 {
			inserted, err := m.database.InsertWaybackRecords(msg.records)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Fetched %d, DB error: %v", len(msg.records), err)
			} else {
				m.statusMsg = fmt.Sprintf("Fetched %d records, %d new inserted", len(msg.records), inserted)
			}
		}
		// Stop stopwatch and load from database
		return m, tea.Batch(m.stopwatch.Stop(), m.loadRecordsFromDB())

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
			m.err = msg.err
			return m, nil
		}
		m.cachedDomains = msg.domains
		m.domainCursor = 0
		m.viewMode = waybackViewDomains
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
						// Resume incomplete fetch
						m.statusMsg = fmt.Sprintf("Resuming fetch from %d records...", fetchState.TotalFetched)
						m.viewMode = waybackViewFetching
						m.fetching = true
						m.fetchProgress = fetchState.TotalFetched
						m.fetchPage = 0
						m.fetchCancelled = false
						m.cancelFetch = make(chan struct{})
						return m, tea.Batch(m.stopwatch.Start(), m.doFetchWithResume(fetchState.ResumeKey))
					}
				}
			}

			// Start fresh fetch
			m.viewMode = waybackViewFetching
			m.fetching = true
			m.fetchProgress = 0
			m.fetchPage = 0
			m.fetchCancelled = false
			m.cancelFetch = make(chan struct{})
			// Start stopwatch and fetch
			return m, tea.Batch(m.stopwatch.Start(), m.doFetch())
		}
		return m, nil

	case "tab":
		// Switch to domain browser
		return m, m.loadCachedDomains()

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
		m.returnToMain = true
		return m, tea.Quit
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
		// Refresh from API
		m.viewMode = waybackViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchPage = 0
		m.cancelFetch = make(chan struct{})
		return m, m.doFetch()

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
	case waybackViewTable:
		contentBuilder.WriteString(m.renderTableView())
	case waybackViewFilter:
		contentBuilder.WriteString(m.renderFilterView())
	case waybackViewDomains:
		contentBuilder.WriteString(m.renderDomainsView())
	case waybackViewDetail:
		contentBuilder.WriteString(m.renderDetailView())
	default:
		// No content for unknown view mode
	}

	// Status message
	if m.statusMsg != "" {
		contentBuilder.WriteString("\n")
		contentBuilder.WriteString(HintStyle.Render(m.statusMsg))
	}

	// Error message
	if m.err != nil {
		contentBuilder.WriteString("\n")
		contentBuilder.WriteString(StatusMsgStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	// Calculate available height
	availableHeight := m.layout.ViewportHeight - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Wrap in bordered box
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n")
	result.WriteString(borderedContent)
	result.WriteString("\n")

	// Help text
	result.WriteString(" " + m.getHelpText())

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
	return b.String()
}

func (m WaybackModel) renderFetchingView() string {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(AccentStyle.Render(fmt.Sprintf("Fetching CDX records for %s...", m.domain)))
	b.WriteString("\n\n")

	// Show progress bar if we have an estimate
	if m.fetchEstimated > 0 {
		percent := float64(m.fetchProgress) / float64(m.fetchEstimated)
		if percent > 1.0 {
			percent = 1.0 // Cap at 100% (we underestimated)
		}
		progressBarWidth := m.layout.InnerWidth - 4
		if progressBarWidth < 40 {
			progressBarWidth = 40
		}
		m.progress.Width = progressBarWidth
		b.WriteString(" ")
		b.WriteString(m.progress.ViewAs(percent))
		b.WriteString("\n\n")
	}

	b.WriteString(ProgressStyle.Render(fmt.Sprintf(" Records: %d  |  Page: %d", m.fetchProgress, m.fetchPage)))
	b.WriteString("\n")
	if m.fetchEstimated > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf(" Estimated: ~%d records", m.fetchEstimated)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(" Press Esc/Q to cancel and save progress"))
	b.WriteString("\n\n")
	// Stopwatch at bottom - using native Bubbles stopwatch View()
	b.WriteString(DimStyle.Render(" Elapsed: "))
	b.WriteString(NormalStyle.Render(m.stopwatch.View()))
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

func (m WaybackModel) getHelpText() string {
	switch m.viewMode {
	case waybackViewInput:
		return HintStyle.Render("Enter: fetch | Tab: browse cached | Esc: back")
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
		// Fetch ALL records using pagination with resume keys
		result := m.client.FetchAllCDXWithResume(m.domain, resumeKey, func(_, _ int) {
			// Progress callback - not used in TUI context (async fetch)
		}, m.cancelFetch)

		// Save fetch state to database
		if m.database != nil {
			lastError := ""
			if result.Error != nil {
				lastError = result.Error.Error()
			}
			_ = m.database.SaveWaybackFetchState(
				m.domain,
				result.ResumeKey,
				len(result.Records),
				result.IsComplete,
				lastError,
			)
		}

		return waybackFetchCompleteMsg{records: result.Records, err: result.Error}
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

// RunWaybackBrowser starts the Wayback Machine CDX browser TUI
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
