package ui

import (
	"fmt"
	"os"
	"sort"
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

// SubdomonsterModel is the TUI model for subdomain enumeration
type SubdomonsterModel struct {
	client    *api.SubdomainClient
	logger    *log.Logger
	database  *db.DB
	layout    Layout
	table     table.Model
	textInput textinput.Model
	spinner   spinner.Model
	progress  progress.Model

	// State
	domain           string
	subdomains       []models.Subdomain
	sortedSubdomains []models.Subdomain // Tree-sorted view
	totalSubdomains  int

	// View mode
	viewMode  subdomonsterViewMode
	inputMode subdomonsterInputMode

	// Filters
	filterText   string
	filterSource string
	filterCDX    int // -1 = all, 0 = not indexed, 1 = indexed

	// Fetch state
	fetching       bool
	fetchProgress  int
	fetchSource    string // "virustotal" or "crtsh"
	cancelFetch    chan struct{}
	fetchCancelled bool
	fetchStartTime time.Time

	// Domain browser state
	cachedDomains []models.TargetDomain
	domainCursor  int

	// Pagination
	page     int
	pageSize int

	// Settings
	vtAPIKey        string
	settingsCursor  int
	settingsEditing bool
	settingsInput   string

	// UI state
	err               error
	statusMsg         string
	quitting          bool
	returnToMain      bool
	layoutInitialized bool
}

type subdomonsterViewMode int

const (
	subdomonsterViewInput    subdomonsterViewMode = iota // Domain input screen
	subdomonsterViewDomains                              // Browse cached domains
	subdomonsterViewFetching                             // Fetching subdomains
	subdomonsterViewTable                                // Table view of subdomains
	subdomonsterViewFilter                               // Filter input overlay
	subdomonsterViewSettings                             // API key settings
)

type subdomonsterInputMode int

const (
	subdomonsterInputDomain subdomonsterInputMode = iota
	subdomonsterInputFilter
	subdomonsterInputAPIKey
)

// Messages
type subdomonsterFetchProgressMsg struct {
	count int
}

type subdomonsterFetchCompleteMsg struct {
	subdomains []models.Subdomain
	source     string
	err        error
}

type subdomonsterDomainsLoadedMsg struct {
	domains []models.TargetDomain
	err     error
}

type subdomonsterSubdomainsLoadedMsg struct {
	subdomains []models.Subdomain
	total      int
	err        error
}

type subdomonsterAPIKeyLoadedMsg struct {
	apiKey string
	err    error
}

// NewSubdomonsterModel creates a new Subdomonster TUI
func NewSubdomonsterModel(logger *log.Logger, database *db.DB) SubdomonsterModel {
	ti := textinput.New()
	ti.Placeholder = "Enter domain (e.g., example.com)"
	ti.Focus()
	ti.CharLimit = 200

	layout := DefaultLayout()

	columns := calculateSubdomonsterColumns(layout.TableWidth - 4)
	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)
	ApplyTableStyles(&t)

	spinnerModel := NewAppSpinner()
	prog := progress.New(
		progress.WithGradient("#FFFFFF", "#FF0000"),
		progress.WithColorProfile(termenv.TrueColor),
	)
	prog.EmptyColor = "241"

	return SubdomonsterModel{
		client:    api.NewSubdomainClient("", nil),
		logger:    logger,
		database:  database,
		layout:    layout,
		table:     t,
		textInput: ti,
		spinner:   spinnerModel,
		progress:  prog,
		viewMode:  subdomonsterViewInput,
		inputMode: subdomonsterInputDomain,
		filterCDX: -1, // Show all by default
		page:      1,
		pageSize:  100,
	}
}

// Init implements tea.Model
func (m SubdomonsterModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick, m.loadCachedDomains(), m.loadAPIKey())
}

// Update implements tea.Model
func (m SubdomonsterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)

		// Calculate height based on actual Subdomonster content
		titleLine := 1
		dividerLine := 1
		spacingLines := 2
		queryInfoLine := 1
		tableNewline := 1
		tableHeaderChrome := 2

		contentOverhead := titleLine + dividerLine + spacingLines + queryInfoLine + tableNewline + tableHeaderChrome
		tableHeight := m.layout.ViewportHeight - TwoBoxOverhead - contentOverhead + 1
		if tableHeight < MinTableHeight {
			tableHeight = MinTableHeight
		}
		m.table.SetHeight(tableHeight)

		const textInputPadding = 10
		m.textInput.Width = m.layout.InnerWidth - textInputPadding

		columns := calculateSubdomonsterColumns(m.layout.TableWidth - 4)
		m.table.SetColumns(columns)
		if len(m.sortedSubdomains) > 0 {
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

	case subdomonsterFetchProgressMsg:
		m.fetchProgress = msg.count
		return m, nil

	case subdomonsterFetchCompleteMsg:
		m.fetching = false
		if msg.err != nil {
			if msg.err.Error() == "cancelled" {
				m.statusMsg = fmt.Sprintf("Fetch cancelled. Got %d subdomains.", len(msg.subdomains))
			} else {
				m.err = msg.err
				m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			}
		} else {
			// Insert subdomains into database
			if m.database != nil && len(msg.subdomains) > 0 {
				inserted, err := m.database.InsertSubdomains(msg.subdomains)
				if err != nil {
					m.err = err
					m.statusMsg = fmt.Sprintf("DB error inserting subdomains: %v", err)
					return m, m.loadSubdomainsFromDB()
				}
				m.statusMsg = fmt.Sprintf("Found %d subdomains (%d new) via %s", len(msg.subdomains), inserted, msg.source)

				// Mark domain as enumerated
				switch msg.source {
				case "virustotal":
					m.database.MarkVTEnumerated(m.domain)
				case "crtsh":
					m.database.MarkCrtshEnumerated(m.domain)
				}
			} else if len(msg.subdomains) == 0 {
				m.statusMsg = fmt.Sprintf("No subdomains found via %s", msg.source)
			}
		}
		return m, m.loadSubdomainsFromDB()

	case subdomonsterDomainsLoadedMsg:
		if msg.err != nil {
			if m.viewMode != subdomonsterViewInput {
				m.err = msg.err
			}
			return m, nil
		}
		m.cachedDomains = msg.domains
		m.domainCursor = 0
		return m, nil

	case subdomonsterSubdomainsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.subdomains = msg.subdomains
		m.totalSubdomains = msg.total
		m.sortSubdomainsTree()
		m.updateTable()
		m.viewMode = subdomonsterViewTable
		return m, nil

	case subdomonsterAPIKeyLoadedMsg:
		if msg.err == nil && msg.apiKey != "" {
			m.vtAPIKey = msg.apiKey
			m.client.SetVirusTotalAPIKey(msg.apiKey)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

func (m SubdomonsterModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.viewMode {
	case subdomonsterViewInput:
		return m.handleInputKeys(msg)
	case subdomonsterViewDomains:
		return m.handleDomainsKeys(msg)
	case subdomonsterViewFetching:
		return m.handleFetchingKeys(msg)
	case subdomonsterViewTable:
		return m.handleTableKeys(msg)
	case subdomonsterViewFilter:
		return m.handleFilterKeys(msg)
	case subdomonsterViewSettings:
		return m.handleSettingsKeys(msg)
	default:
		return m, nil
	}
}

func (m SubdomonsterModel) handleInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.quitting = true
		m.returnToMain = true
		return m, tea.Quit

	case "enter":
		if m.textInput.Value() != "" {
			domain := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
			m.domain = domain
			m.err = nil

			// Ensure target domain exists in database
			if m.database != nil {
				m.database.InsertTargetDomain(domain)
			}

			// Load existing subdomains
			return m, m.loadSubdomainsFromDB()
		}
		return m, nil

	case "tab":
		// Switch to domain browser
		m.viewMode = subdomonsterViewDomains
		return m, m.loadCachedDomains()

	case "ctrl+s":
		// Open settings
		m.viewMode = subdomonsterViewSettings
		m.settingsCursor = 0
		m.settingsEditing = false
		m.settingsInput = m.vtAPIKey
		return m, nil

	case "v":
		// Enumerate via VirusTotal directly from input view
		if m.textInput.Value() == "" {
			m.statusMsg = "Enter a domain first"
			return m, nil
		}
		if !m.client.HasVirusTotalAPIKey() {
			// Prompt for API key
			m.viewMode = subdomonsterViewSettings
			m.settingsEditing = true
			m.settingsInput = ""
			m.statusMsg = "Enter your VirusTotal API key:"
			return m, nil
		}
		// Set domain and ensure it exists in database
		domain := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
		m.domain = domain
		m.err = nil
		if m.database != nil {
			m.database.InsertTargetDomain(domain)
		}
		// Start VirusTotal fetch
		m.viewMode = subdomonsterViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchSource = "virustotal"
		m.fetchCancelled = false
		m.cancelFetch = make(chan struct{})
		m.fetchStartTime = time.Now()
		m.statusMsg = "Fetching subdomains from VirusTotal..."
		return m, tea.Batch(m.progress.SetPercent(0.0), m.doVirusTotalFetch())

	case "c":
		// Enumerate via crt.sh directly from input view
		if m.textInput.Value() == "" {
			m.statusMsg = "Enter a domain first"
			return m, nil
		}
		// Set domain and ensure it exists in database
		domain := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
		m.domain = domain
		m.err = nil
		if m.database != nil {
			m.database.InsertTargetDomain(domain)
		}
		// Start crt.sh fetch
		m.viewMode = subdomonsterViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchSource = "crtsh"
		m.fetchCancelled = false
		m.cancelFetch = make(chan struct{})
		m.fetchStartTime = time.Now()
		m.statusMsg = "Fetching subdomains from crt.sh..."
		return m, tea.Batch(m.progress.SetPercent(0.0), m.doCrtshFetch())

	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m SubdomonsterModel) handleDomainsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.viewMode = subdomonsterViewInput
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
			return m, m.loadSubdomainsFromDB()
		}

	case "d", "D":
		// Delete selected domain
		if len(m.cachedDomains) > 0 && m.domainCursor < len(m.cachedDomains) && m.database != nil {
			domain := m.cachedDomains[m.domainCursor].Domain
			if err := m.database.DeleteTargetDomain(domain); err != nil {
				m.statusMsg = fmt.Sprintf("Delete error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("Deleted domain: %s", domain)
				return m, m.loadCachedDomains()
			}
		}

	case "a", "A":
		// Add new domain - go to input view
		m.viewMode = subdomonsterViewInput
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m SubdomonsterModel) handleFetchingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// Cancel fetch
		if !m.fetchCancelled && m.cancelFetch != nil {
			close(m.cancelFetch)
			m.fetchCancelled = true
		}
		m.fetching = false
		return m, m.loadSubdomainsFromDB()
	}
	return m, nil
}

func (m SubdomonsterModel) handleTableKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.viewMode = subdomonsterViewInput
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "up", "k":
		m.table.MoveUp(1)
		return m, nil

	case "down", "j":
		m.table.MoveDown(1)
		return m, nil

	case "v":
		// Enumerate via VirusTotal
		if !m.client.HasVirusTotalAPIKey() {
			// Prompt for API key
			m.viewMode = subdomonsterViewSettings
			m.settingsEditing = true
			m.settingsInput = ""
			m.statusMsg = "Enter your VirusTotal API key:"
			return m, nil
		}
		m.viewMode = subdomonsterViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchSource = "virustotal"
		m.fetchCancelled = false
		m.cancelFetch = make(chan struct{})
		m.fetchStartTime = time.Now()
		m.statusMsg = "Fetching subdomains from VirusTotal..."
		return m, tea.Batch(m.progress.SetPercent(0.0), m.doVirusTotalFetch())

	case "c":
		// Enumerate via crt.sh
		m.viewMode = subdomonsterViewFetching
		m.fetching = true
		m.fetchProgress = 0
		m.fetchSource = "crtsh"
		m.fetchCancelled = false
		m.cancelFetch = make(chan struct{})
		m.fetchStartTime = time.Now()
		m.statusMsg = "Fetching subdomains from crt.sh..."
		return m, tea.Batch(m.progress.SetPercent(0.0), m.doCrtshFetch())

	case "i":
		// Import JSON file
		m.statusMsg = "Import: Use 'gitsome-ng import-subdomains <file.json> <domain>' from command line"
		return m, nil

	case "W":
		// Forward to Wayback (mark as CDX indexed - roadmap placeholder)
		if len(m.sortedSubdomains) > 0 {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.sortedSubdomains) {
				subdomain := m.sortedSubdomains[cursor].Subdomain
				if m.database != nil {
					m.database.MarkSubdomainCDXIndexed(subdomain)
					m.statusMsg = fmt.Sprintf("Marked %s for Wayback processing (roadmap feature)", subdomain)
					return m, m.loadSubdomainsFromDB()
				}
			}
		}
		return m, nil

	case "/":
		// Enter filter mode
		m.viewMode = subdomonsterViewFilter
		m.inputMode = subdomonsterInputFilter
		m.textInput.SetValue(m.filterText)
		m.textInput.Placeholder = "Filter by subdomain..."
		m.textInput.Focus()
		return m, textinput.Blink

	case "f":
		// Cycle source filter
		switch m.filterSource {
		case "":
			m.filterSource = "virustotal"
		case "virustotal":
			m.filterSource = "crtsh"
		case "crtsh":
			m.filterSource = "import"
		case "import":
			m.filterSource = ""
		}
		m.page = 1
		m.statusMsg = fmt.Sprintf("Filter: source=%s", m.filterSource)
		if m.filterSource == "" {
			m.statusMsg = "Filter: showing all sources"
		}
		return m, m.loadSubdomainsFromDB()

	case "x":
		// Cycle CDX filter
		switch m.filterCDX {
		case -1:
			m.filterCDX = 0 // Not indexed
		case 0:
			m.filterCDX = 1 // Indexed
		case 1:
			m.filterCDX = -1 // All
		}
		m.page = 1
		switch m.filterCDX {
		case -1:
			m.statusMsg = "Filter: showing all CDX status"
		case 0:
			m.statusMsg = "Filter: showing NOT CDX indexed"
		case 1:
			m.statusMsg = "Filter: showing CDX indexed"
		}
		return m, m.loadSubdomainsFromDB()

	case "r":
		// Clear filters and reload
		m.filterText = ""
		m.filterSource = ""
		m.filterCDX = -1
		m.page = 1
		m.statusMsg = "Filters cleared"
		return m, m.loadSubdomainsFromDB()

	case "e":
		// Export to markdown
		if len(m.sortedSubdomains) > 0 {
			filename := fmt.Sprintf("subdomains-%s-%s.md", m.domain, time.Now().Format("20060102-150405"))
			if err := m.exportToMarkdown(filename); err != nil {
				m.statusMsg = fmt.Sprintf("Export error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("Exported to %s", filename)
			}
		}
		return m, nil

	case "d":
		// Delete selected subdomain
		if len(m.sortedSubdomains) > 0 && m.database != nil {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.sortedSubdomains) {
				subdomain := m.sortedSubdomains[cursor]
				if err := m.database.DeleteSubdomain(subdomain.ID); err != nil {
					m.statusMsg = fmt.Sprintf("Delete error: %v", err)
				} else {
					m.statusMsg = "Subdomain deleted"
					return m, m.loadSubdomainsFromDB()
				}
			}
		}
		return m, nil

	case "ctrl+s":
		// Open settings
		m.viewMode = subdomonsterViewSettings
		m.settingsCursor = 0
		m.settingsEditing = false
		m.settingsInput = m.vtAPIKey
		return m, nil

	case "n", "right":
		// Next page
		maxPage := (m.totalSubdomains + m.pageSize - 1) / m.pageSize
		if m.page < maxPage {
			m.page++
			return m, m.loadSubdomainsFromDB()
		}

	case "p", "left":
		// Previous page
		if m.page > 1 {
			m.page--
			return m, m.loadSubdomainsFromDB()
		}
	}
	return m, nil
}

func (m SubdomonsterModel) handleFilterKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filterText = m.textInput.Value()
		m.page = 1
		m.viewMode = subdomonsterViewTable
		m.textInput.Placeholder = "Enter domain (e.g., example.com)"
		return m, m.loadSubdomainsFromDB()

	case "esc":
		m.viewMode = subdomonsterViewTable
		m.textInput.Placeholder = "Enter domain (e.g., example.com)"
		return m, nil

	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m SubdomonsterModel) handleSettingsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsEditing {
		switch msg.String() {
		case "esc":
			m.settingsEditing = false
			m.settingsInput = m.vtAPIKey
			return m, nil

		case "enter":
			m.vtAPIKey = m.settingsInput
			m.client.SetVirusTotalAPIKey(m.vtAPIKey)
			m.settingsEditing = false
			// Save to database for persistence
			if m.database != nil {
				if err := m.database.SetVirusTotalAPIKey(m.vtAPIKey); err != nil {
					m.statusMsg = fmt.Sprintf("API key set but failed to save: %v", err)
				} else {
					m.statusMsg = "VirusTotal API key saved. Press V to fetch."
				}
			} else {
				m.statusMsg = "VirusTotal API key set (not persisted - no database)"
			}
			// Return to table view
			if m.domain != "" {
				m.viewMode = subdomonsterViewTable
			} else {
				m.viewMode = subdomonsterViewInput
			}
			return m, nil

		case "backspace":
			if len(m.settingsInput) > 0 {
				m.settingsInput = m.settingsInput[:len(m.settingsInput)-1]
			}
			return m, nil

		default:
			if len(msg.String()) == 1 {
				m.settingsInput += msg.String()
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "esc", "q":
		if m.domain != "" {
			m.viewMode = subdomonsterViewTable
		} else {
			m.viewMode = subdomonsterViewInput
			return m, textinput.Blink
		}
		return m, nil

	case "enter", "e":
		m.settingsEditing = true
		m.settingsInput = m.vtAPIKey
		return m, nil
	}
	return m, nil
}

// View implements tea.Model
func (m SubdomonsterModel) View() string {
	if m.quitting {
		return ""
	}

	// Use PageViewBuilder to ensure consistent layout structure matching Layout.TableHeight assumptions
	builder := NewPageView(m.layout).
		Title("  SubDomonster - Subdomain Enumeration").
		Divider().
		Spacing(2)

	var viewContent string
	switch m.viewMode {
	case subdomonsterViewInput:
		viewContent = m.renderInputView()
	case subdomonsterViewDomains:
		viewContent = m.renderDomainsView()
	case subdomonsterViewFetching:
		viewContent = m.renderFetchingView()
	case subdomonsterViewTable:
		viewContent = m.renderTableView()
	case subdomonsterViewFilter:
		viewContent = m.renderFilterView()
	case subdomonsterViewSettings:
		viewContent = m.renderSettingsView()
	}

	builder.CustomContent(viewContent)

	// Error message (if any)
	if m.err != nil {
		builder.Error(m.err)
	}

	return builder.Help(m.getHelpText()).Build()
}

func (m SubdomonsterModel) renderInputView() string {
	var b strings.Builder
	b.WriteString(" Domain: ")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(HintStyle.Render(" Enter a root domain to enumerate subdomains."))
	b.WriteString("\n")
	b.WriteString(HintStyle.Render(" Example: example.com"))
	b.WriteString("\n\n")
	b.WriteString(HintStyle.Render(" Press Tab to browse cached domains."))

	// Show recently cached domains
	if len(m.cachedDomains) > 0 {
		b.WriteString("\n\n")
		b.WriteString(NormalStyle.Render(" Recently tracked domains:"))
		b.WriteString("\n")

		maxShow := 10
		if len(m.cachedDomains) < maxShow {
			maxShow = len(m.cachedDomains)
		}
		for i := 0; i < maxShow; i++ {
			d := m.cachedDomains[i]
			line := fmt.Sprintf("   %s (%d subdomains)", d.Domain, d.SubdomainCount)
			b.WriteString(NormalStyle.Render(line))
			b.WriteString("\n")
		}
		if len(m.cachedDomains) > maxShow {
			more := len(m.cachedDomains) - maxShow
			b.WriteString(NormalStyle.Render(fmt.Sprintf("   ... and %d more (press Tab to browse)", more)))
			b.WriteString("\n")
		}
	}

	// Status message
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(NormalStyle.Render(" " + m.statusMsg))
	}

	return b.String()
}

func (m SubdomonsterModel) renderDomainsView() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(" Tracked Domains"))
	b.WriteString("\n\n")

	if len(m.cachedDomains) == 0 {
		b.WriteString(HintStyle.Render(" No tracked domains. Enter a domain to start."))
		return b.String()
	}

	selectedStyle := SelectedStyle.Width(m.layout.InnerWidth)
	normalStyle := NormalStyle.Width(m.layout.InnerWidth)

	for i, d := range m.cachedDomains {
		line := fmt.Sprintf("%s (%d subdomains)", d.Domain, d.SubdomainCount)
		if i == m.domainCursor {
			b.WriteString(selectedStyle.Render("> " + line))
		} else {
			b.WriteString(normalStyle.Render("  " + line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m SubdomonsterModel) renderFetchingView() string {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(AccentStyle.Render(fmt.Sprintf("Fetching subdomains for %s via %s...", m.domain, m.fetchSource)))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render(fmt.Sprintf(" Subdomains found: %d", m.fetchProgress)))
	b.WriteString("\n")

	// Elapsed time
	if !m.fetchStartTime.IsZero() {
		elapsed := time.Since(m.fetchStartTime)
		b.WriteString(NormalStyle.Render(fmt.Sprintf(" Elapsed: %.1fs", elapsed.Seconds())))
	}
	b.WriteString("\n\n")

	b.WriteString(HintStyle.Render(" Esc: cancel"))

	// Status message
	if m.statusMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(NormalStyle.Render(" " + m.statusMsg))
	}

	return b.String()
}

func (m SubdomonsterModel) renderTableView() string {
	// Build query info
	queryInfo := fmt.Sprintf(" Domain: %s", m.domain)
	if m.filterText != "" || m.filterSource != "" || m.filterCDX != -1 {
		queryInfo += "  |  Filters:"
		if m.filterText != "" {
			queryInfo += fmt.Sprintf(" '%s'", m.filterText)
		}
		if m.filterSource != "" {
			queryInfo += fmt.Sprintf(" src=%s", m.filterSource)
		}
		switch m.filterCDX {
		case 0:
			queryInfo += " CDX=no"
		case 1:
			queryInfo += " CDX=yes"
		}
	}
	maxPage := (m.totalSubdomains + m.pageSize - 1) / m.pageSize
	if maxPage < 1 {
		maxPage = 1
	}
	currentRow := m.table.Cursor() + 1
	totalRows := len(m.sortedSubdomains)
	queryInfo += fmt.Sprintf("  |  Page %d/%d  |  Total: %d  |  Row %d/%d", m.page, maxPage, m.totalSubdomains, currentRow, totalRows)

	// Use PageViewBuilder for consistent rendering (matches wayback.go pattern)
	builder := NewPageView(m.layout).
		QueryInfo(queryInfo).
		Table(m.table)

	return builder.BuildContent()
}

func (m SubdomonsterModel) renderFilterView() string {
	var b strings.Builder
	b.WriteString(m.renderTableView())
	b.WriteString("\n\n")
	b.WriteString(AccentStyle.Render(" Filter: "))
	b.WriteString(m.textInput.View())
	return b.String()
}

func (m SubdomonsterModel) renderSettingsView() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(" Settings"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render(" VirusTotal API Key:"))
	b.WriteString("\n")

	if m.settingsEditing {
		// Show masked input
		masked := strings.Repeat("*", len(m.settingsInput))
		if len(masked) > 40 {
			masked = masked[:40] + "..."
		}
		b.WriteString(AccentStyle.Render("   " + masked + "_"))
	} else {
		if m.vtAPIKey != "" {
			// Show masked key
			masked := strings.Repeat("*", len(m.vtAPIKey))
			if len(masked) > 40 {
				masked = masked[:40] + "..."
			}
			b.WriteString(NormalStyle.Render("   " + masked))
		} else {
			b.WriteString(HintStyle.Render("   (not set)"))
		}
	}
	b.WriteString("\n\n")

	b.WriteString(HintStyle.Render(" Get your API key from: https://www.virustotal.com/gui/my-apikey"))
	b.WriteString("\n\n")

	if m.settingsEditing {
		b.WriteString(HintStyle.Render(" Enter: save | Esc: cancel"))
	} else {
		b.WriteString(HintStyle.Render(" e/Enter: edit | Esc: back"))
	}

	return b.String()
}

func (m SubdomonsterModel) getHelpText() string {
	switch m.viewMode {
	case subdomonsterViewInput:
		return "Enter: search | v: VirusTotal | c: crt.sh | Tab: browse cached | Ctrl-S: settings | Esc: back"
	case subdomonsterViewDomains:
		return "Enter: select | a: add domain | d: delete domain | j/k: navigate | Esc: back"
	case subdomonsterViewFetching:
		return "Esc: cancel fetch"
	case subdomonsterViewTable:
		return "v: VirusTotal | c: crt.sh | /: search | f: filter source | x: toggle CDX | e: export | Esc: back"
	case subdomonsterViewFilter:
		return "Enter: apply filter | Esc: cancel"
	case subdomonsterViewSettings:
		if m.settingsEditing {
			return "Enter: save | Esc: cancel"
		}
		return "e: edit API key | Esc: back"
	default:
		return ""
	}
}

// =============================================================================
// Commands
// =============================================================================

func (m SubdomonsterModel) loadCachedDomains() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return subdomonsterDomainsLoadedMsg{err: fmt.Errorf("no database")}
		}
		domains, err := m.database.GetTargetDomainsWithCounts()
		return subdomonsterDomainsLoadedMsg{domains: domains, err: err}
	}
}

func (m SubdomonsterModel) loadAPIKey() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return subdomonsterAPIKeyLoadedMsg{err: fmt.Errorf("no database")}
		}
		apiKey, err := m.database.GetVirusTotalAPIKey()
		return subdomonsterAPIKeyLoadedMsg{apiKey: apiKey, err: err}
	}
}

func (m SubdomonsterModel) loadSubdomainsFromDB() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return subdomonsterSubdomainsLoadedMsg{err: fmt.Errorf("no database")}
		}

		filter := models.SubdomainFilter{
			Domain:     m.domain,
			SearchText: m.filterText,
			Source:     m.filterSource,
			CDXIndexed: m.filterCDX,
			Limit:      m.pageSize,
			Offset:     (m.page - 1) * m.pageSize,
		}

		subdomains, total, err := m.database.GetSubdomainsFiltered(filter)
		return subdomonsterSubdomainsLoadedMsg{subdomains: subdomains, total: total, err: err}
	}
}

func (m SubdomonsterModel) doVirusTotalFetch() tea.Cmd {
	return func() tea.Msg {
		subdomains, err := m.client.FetchAllVirusTotalSubdomains(
			m.domain,
			func(count int) {
				// Progress callback - not directly usable in Bubble Tea
			},
			m.cancelFetch,
		)
		return subdomonsterFetchCompleteMsg{
			subdomains: subdomains,
			source:     "virustotal",
			err:        err,
		}
	}
}

func (m SubdomonsterModel) doCrtshFetch() tea.Cmd {
	return func() tea.Msg {
		subdomains, err := m.client.FetchCrtshSubdomains(m.domain)
		return subdomonsterFetchCompleteMsg{
			subdomains: subdomains,
			source:     "crtsh",
			err:        err,
		}
	}
}

// =============================================================================
// Tree Sort
// =============================================================================

// sortSubdomainsTree sorts subdomains in reverse-tree order
// This groups subdomains by their hierarchy from right to left
// e.g., db.stage.example.com and web.stage.example.com are grouped together
func (m *SubdomonsterModel) sortSubdomainsTree() {
	m.sortedSubdomains = make([]models.Subdomain, len(m.subdomains))
	copy(m.sortedSubdomains, m.subdomains)

	sort.Slice(m.sortedSubdomains, func(i, j int) bool {
		return compareSubdomainsTree(m.sortedSubdomains[i].Subdomain, m.sortedSubdomains[j].Subdomain)
	})
}

// compareSubdomainsTree compares two subdomains for tree sorting
// Returns true if a should come before b
func compareSubdomainsTree(a, b string) bool {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	// Reverse the parts for comparison
	reverseStrings(partsA)
	reverseStrings(partsB)

	// Compare part by part
	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}

	for i := 0; i < minLen; i++ {
		if partsA[i] != partsB[i] {
			return partsA[i] < partsB[i]
		}
	}

	// Shorter subdomain comes first
	return len(partsA) < len(partsB)
}

func reverseStrings(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// =============================================================================
// Table Rendering
// =============================================================================

func (m *SubdomonsterModel) updateTable() {
	oldCursor := m.table.Cursor()

	columns := calculateSubdomonsterColumns(m.layout.TableWidth - 4)

	subdomainW := columns[0].Width
	sourceW := columns[1].Width
	// cdxW and expiredW not needed - they use fixed checkbox format

	truncate := func(s string, w int) string {
		if len(s) <= w {
			return s
		}
		if w <= 3 {
			return s[:w]
		}
		return s[:w-3] + "..."
	}

	rows := make([]table.Row, len(m.sortedSubdomains))
	for i, s := range m.sortedSubdomains {
		cdxStatus := "[ ]"
		if s.CDXIndexed {
			cdxStatus = "[x]"
		}

		expiredStatus := "[ ]"
		if s.CertExpired {
			expiredStatus = "[x]"
		}

		rows[i] = table.Row{
			truncate(s.Subdomain, subdomainW),
			truncate(s.Source, sourceW),
			cdxStatus,
			expiredStatus,
		}
	}

	m.table.SetColumns(columns)
	m.table.SetRows(rows)
	m.table.SetCursor(oldCursor)
}

const (
	subdomonsterSourceWidth  = 12
	subdomonsterCDXWidth     = 5
	subdomonsterExpiredWidth = 9
	subdomonsterMinSubWidth  = 40
	subdomonsterMinTotal     = 70
)

func calculateSubdomonsterColumns(totalW int) []table.Column {
	if totalW < subdomonsterMinTotal {
		totalW = subdomonsterMinTotal
	}

	fixedTotal := subdomonsterSourceWidth + subdomonsterCDXWidth + subdomonsterExpiredWidth

	// Subdomain gets remaining space
	subdomainW := totalW - fixedTotal
	if subdomainW < subdomonsterMinSubWidth {
		subdomainW = subdomonsterMinSubWidth
	}

	// Verify exact match
	actualTotal := subdomainW + subdomonsterSourceWidth + subdomonsterCDXWidth + subdomonsterExpiredWidth
	if actualTotal != totalW {
		subdomainW += (totalW - actualTotal)
	}

	return []table.Column{
		{Title: "Subdomain", Width: subdomainW},
		{Title: "Source", Width: subdomonsterSourceWidth},
		{Title: "CDX", Width: subdomonsterCDXWidth},
		{Title: "Expired  ", Width: subdomonsterExpiredWidth},
	}
}

// =============================================================================
// Export
// =============================================================================

func (m SubdomonsterModel) exportToMarkdown(filename string) error {
	var b strings.Builder

	// Get stats
	var stats *models.SubdomainStats
	if m.database != nil {
		stats, _ = m.database.GetSubdomainStats(m.domain)
	}

	b.WriteString(fmt.Sprintf("# Subdomains: %s\n\n", m.domain))
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	if stats != nil {
		b.WriteString("## Summary\n\n")
		b.WriteString(fmt.Sprintf("- Total: %d\n", stats.Total))
		b.WriteString(fmt.Sprintf("- VirusTotal: %d\n", stats.VTCount))
		b.WriteString(fmt.Sprintf("- crt.sh: %d\n", stats.CrtshCount))
		b.WriteString(fmt.Sprintf("- Import: %d\n", stats.ImportCount))
		b.WriteString(fmt.Sprintf("- CDX Indexed: %d\n", stats.CDXCount))
		b.WriteString(fmt.Sprintf("- Expired Certs: %d\n", stats.ExpiredCount))
		b.WriteString("\n")
	}

	b.WriteString("## Subdomains\n\n")
	b.WriteString("| Subdomain | Source | CDX | Expired |\n")
	b.WriteString("|-----------|--------|-----|--------|\n")

	// Get all subdomains for export
	var allSubdomains []models.Subdomain
	if m.database != nil {
		allSubdomains, _ = m.database.GetAllSubdomainsForDomain(m.domain)
	} else {
		allSubdomains = m.sortedSubdomains
	}

	for _, s := range allSubdomains {
		cdx := "[ ]"
		if s.CDXIndexed {
			cdx = "[x]"
		}
		expired := "[ ]"
		if s.CertExpired {
			expired = "[x]"
		}
		// Escape pipes in values
		subdomain := strings.ReplaceAll(s.Subdomain, "|", "\\|")

		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			subdomain, s.Source, cdx, expired))
	}

	return os.WriteFile(filename, []byte(b.String()), 0644)
}

// =============================================================================
// Public API
// =============================================================================

// ShouldReturnToMain returns true if user wants to go back
func (m SubdomonsterModel) ShouldReturnToMain() bool {
	return m.returnToMain
}

// RunSubdomonster starts the Subdomonster TUI
func RunSubdomonster(logger *log.Logger, database *db.DB) error {
	model := NewSubdomonsterModel(logger, database)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(SubdomonsterModel)
	if !ok {
		return nil
	}

	if m.ShouldReturnToMain() {
		return nil
	}

	return nil
}

// RunSubdomonsterCache starts the Subdomonster TUI directly in cached domains browser mode
func RunSubdomonsterCache(logger *log.Logger, database *db.DB) error {
	model := NewSubdomonsterModel(logger, database)
	// Start directly in domains view
	model.viewMode = subdomonsterViewDomains
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := finalModel.(SubdomonsterModel)
	if !ok {
		return nil
	}

	if m.ShouldReturnToMain() {
		return nil
	}

	return nil
}
