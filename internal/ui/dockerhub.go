package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
)

// DockerHubSearchModel is the TUI model for Docker Hub search
type DockerHubSearchModel struct {
	client            *api.DockerHubClient
	logger            *log.Logger
	database          *db.DB
	layout            Layout
	table             table.Model
	textInput         textinput.Model
	spinner           spinner.Model
	results           *api.DockerHubSearchResponse
	query             string
	page              int
	searching         bool
	inputMode         bool
	err               error
	quitting          bool
	returnToMain      bool
	launchInspector   bool           // true when user wants to inspect an image
	selectedImage     string         // image name to inspect
	cachedImages      map[string]int // image name -> layer count (from DB)
	pendingSearch     bool           // true when initial search should be triggered on Init
	layoutInitialized bool           // true after first WindowSizeMsg received
}

// DockerHubSearchMsg is sent when search results are ready
type DockerHubSearchMsg struct {
	Results *api.DockerHubSearchResponse
	Err     error
}

// NewDockerHubSearchModel creates a new Docker Hub search TUI
func NewDockerHubSearchModel(logger *log.Logger, database *db.DB) DockerHubSearchModel {
	// Create text input for search
	ti := textinput.New()
	ti.Placeholder = "Enter search term..."
	ti.Focus()
	ti.CharLimit = 100
	// ti.Width is set dynamically in Update() on tea.WindowSizeMsg

	// Calculate layout
	layout := DefaultLayout()

	// Calculate initial column widths to fill TableWidth (accounts for bubbles overhead)
	columns := calculateDockerHubColumns(layout.TableWidth)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	// Apply standard table styles (same as layerslayer.go)
	ApplyTableStyles(&t)

	// Create red spinner for search operations
	spinnerModel := NewAppSpinner()

	return DockerHubSearchModel{
		client:       api.NewDockerHubClient(logger),
		logger:       logger,
		database:     database,
		layout:       layout,
		table:        t,
		textInput:    ti,
		spinner:      spinnerModel,
		page:         1,
		inputMode:    true,
		cachedImages: make(map[string]int),
	}
}

// Init implements tea.Model
func (m DockerHubSearchModel) Init() tea.Cmd {
	// Don't trigger pendingSearch here - wait for WindowSizeMsg first
	// so that layout is properly initialized with correct terminal dimensions
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

// Update implements tea.Model
func (m DockerHubSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.table.SetHeight(m.layout.TableHeight)
		// Update text input width dynamically
		m.textInput.Width = m.layout.InnerWidth - 10
		// Always update column widths on resize (for full-width selector highlighting)
		columns := calculateDockerHubColumns(m.layout.TableWidth)
		m.table.SetColumns(columns)
		if m.results != nil {
			m.updateTable()
		}

		// Trigger pending search after layout is initialized with correct dimensions
		if m.pendingSearch && !m.layoutInitialized {
			m.layoutInitialized = true
			m.pendingSearch = false // Clear flag so it doesn't re-trigger
			return m, m.doSearch()
		}
		m.layoutInitialized = true
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case DockerHubSearchMsg:
		m.searching = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.results = msg.Results
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		// Handle input mode
		if m.inputMode {
			switch msg.String() {
			case "enter":
				if m.textInput.Value() != "" {
					m.query = m.textInput.Value()
					m.page = 1
					m.inputMode = false
					m.searching = true
					return m, m.doSearch()
				}
			case "esc":
				m.quitting = true
				m.returnToMain = true
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Handle table mode
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			m.returnToMain = true
			return m, tea.Quit

		case "/":
			// Enter search mode
			m.inputMode = true
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, textinput.Blink

		case "n", "right":
			// Next page
			if m.results != nil && m.page*25 < m.results.Total {
				m.page++
				m.searching = true
				return m, m.doSearch()
			}

		case "p", "left":
			// Previous page
			if m.page > 1 {
				m.page--
				m.searching = true
				return m, m.doSearch()
			}

		case "up", "k":
			m.table.MoveUp(1)
			return m, nil

		case "down", "j":
			m.table.MoveDown(1)
			return m, nil

		case "enter":
			// Launch layer inspector for selected image
			fmt.Println("DEBUG: Enter pressed in table mode")
			if m.results != nil && len(m.results.Results) > 0 {
				cursor := m.table.Cursor()
				fmt.Printf("DEBUG: cursor=%d, results=%d\n", cursor, len(m.results.Results))
				if cursor >= 0 && cursor < len(m.results.Results) {
					selected := m.results.Results[cursor]
					m.selectedImage = selected.Name
					m.launchInspector = true
					m.quitting = true
					fmt.Printf("DEBUG: selectedImage=%s, launchInspector=%v\n", m.selectedImage, m.launchInspector)
					return m, tea.Quit
				}
			}
		}
	}

	return m, nil
}

// View implements tea.Model
func (m DockerHubSearchModel) View() string {
	if m.quitting {
		return ""
	}

	// Build content inside border
	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Docker Hub Search"))
	contentBuilder.WriteString("\n")
	// White divider after title (use InnerWidth to fit within bordered content area)
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	// Search input or current query
	if m.inputMode {
		contentBuilder.WriteString(" Search: ")
		contentBuilder.WriteString(m.textInput.View())
	} else {
		// Show current query and pagination
		queryInfo := fmt.Sprintf(" Query: %s", m.query)
		if m.results != nil {
			queryInfo += fmt.Sprintf("  |  Page %d  |  Total: %d results", m.page, m.results.Total)
		}
		contentBuilder.WriteString(AccentStyle.Render(queryInfo))
		contentBuilder.WriteString("\n\n")
		// NOTE: No manual divider here - the bubbles/table header has BorderBottom(true)
		// which renders its own divider line under the column headers

		if m.searching {
			// Show red spinner when searching
			contentBuilder.WriteString(m.spinner.View())
			contentBuilder.WriteString(" ")
			contentBuilder.WriteString(HintStyle.Render("Searching..."))
		} else if m.err != nil {
			// Use predefined style instead of creating new one
			contentBuilder.WriteString(StatusMsgStyle.Render(fmt.Sprintf(" Error: %v", m.err)))
		} else {
			// Render table directly (no custom styling - let bubbles handle it)
			contentBuilder.WriteString(m.table.View())
		}
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

	// Second box: Help text (white border, 1 row high)
	var helpText string
	if m.inputMode {
		helpText = "Enter: search | Esc: back to main"
	} else {
		helpText = "Enter: inspect | /: search | n: next | p: prev | Esc: back"
	}
	textWidth := len(helpText)
	padding := (m.layout.InnerWidth - textWidth) / 2
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

// doSearch performs the search asynchronously
func (m DockerHubSearchModel) doSearch() tea.Cmd {
	return func() tea.Msg {
		results, err := m.client.Search(m.query, m.page)
		return DockerHubSearchMsg{Results: results, Err: err}
	}
}

// calculateDockerHubColumns calculates column widths to fill the given width
// This ensures the selector highlight spans the full width
func calculateDockerHubColumns(totalW int) []table.Column {
	if totalW < 50 {
		totalW = 50
	}

	// Fixed column widths - keep small to leave room for Description
	nameW := 14
	publisherW := 8
	starsW := 5
	pullsW := 5
	badgeW := 8
	fixedTotal := nameW + publisherW + starsW + pullsW + badgeW // = 40

	// Description gets remaining space - ensure columns sum exactly to totalW
	descW := totalW - fixedTotal
	if descW < 10 {
		// If description too small, shrink other columns proportionally
		descW = 10
		shrink := fixedTotal + descW - totalW
		// Shrink name column (largest flexible column)
		if nameW > shrink {
			nameW -= shrink
		}
	}

	// Verify columns sum to totalW exactly
	actualTotal := nameW + publisherW + starsW + pullsW + badgeW + descW
	if actualTotal != totalW {
		// Adjust description to make exact match
		descW += (totalW - actualTotal)
	}

	return []table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Publisher", Width: publisherW},
		{Title: "Stars", Width: starsW},
		{Title: "Pulls", Width: pullsW},
		{Title: "Badge", Width: badgeW},
		{Title: "Description", Width: descW},
	}
}

// updateTable updates the table with search results
func (m *DockerHubSearchModel) updateTable() {
	if m.results == nil {
		return
	}

	// Check which images we have cached layer data for
	m.cachedImages = make(map[string]int)
	if m.database != nil {
		for _, r := range m.results.Results {
			// Check all possible tag variants (we store with tag in image_ref)
			inspections, err := m.database.GetLayerInspectionsByImage(r.Name + ":latest")
			if err == nil && len(inspections) > 0 {
				m.cachedImages[r.Name] = len(inspections)
			} else {
				// Also check without tag (partial match would need different query)
				// For now, check common tags
				for _, tag := range []string{"1", "latest", "main", "master"} {
					inspections, err := m.database.GetLayerInspectionsByImage(r.Name + ":" + tag)
					if err == nil && len(inspections) > 0 {
						m.cachedImages[r.Name] = len(inspections)
						break
					}
				}
			}
		}
	}

	// Get column widths from central calculation function (uses TableWidth for bubbles overhead)
	columns := calculateDockerHubColumns(m.layout.TableWidth)

	// Extract widths for truncation
	nameW := columns[0].Width
	publisherW := columns[1].Width
	starsW := columns[2].Width
	pullsW := columns[3].Width
	badgeW := columns[4].Width
	descW := columns[5].Width

	// Helper to truncate string to width
	truncate := func(s string, w int) string {
		if len(s) <= w {
			return s
		}
		if w <= 3 {
			return s[:w]
		}
		return s[:w-3] + "..."
	}

	rows := make([]table.Row, len(m.results.Results))
	for i, r := range m.results.Results {
		desc := truncate(r.ShortDescription, descW)

		var badge string
		switch r.Badge {
		case "verified_publisher":
			badge = "Verified"
		case "official":
			badge = "Official"
		}

		// Add cached indicator to name if we have layer data
		name := r.Name
		if layerCount, ok := m.cachedImages[r.Name]; ok {
			name = fmt.Sprintf("[%d] %s", layerCount, r.Name)
		}
		name = truncate(name, nameW)
		publisher := truncate(r.Publisher, publisherW)
		stars := truncate(fmt.Sprintf("%d", r.StarCount), starsW)
		pulls := truncate(r.PullCount, pullsW)
		badge = truncate(badge, badgeW)

		rows[i] = table.Row{
			name,
			publisher,
			stars,
			pulls,
			badge,
			desc,
		}
	}

	// Set columns and rows
	m.table.SetColumns(columns)
	m.table.SetRows(rows)
}

// ShouldReturnToMain returns true if user wants to go back
func (m DockerHubSearchModel) ShouldReturnToMain() bool {
	return m.returnToMain
}

// ShouldLaunchInspector returns true if user wants to inspect an image
func (m DockerHubSearchModel) ShouldLaunchInspector() bool {
	return m.launchInspector
}

// SelectedImage returns the image name selected for inspection
func (m DockerHubSearchModel) SelectedImage() string {
	return m.selectedImage
}

// RunDockerHubSearch starts the Docker Hub search TUI
// initialQuery can be provided to pre-fill the search and trigger an immediate search (e.g., from DockerHub profile redirect)
func RunDockerHubSearch(logger *log.Logger, database *db.DB, initialQuery string) error {
	// Keep track of the last search state to restore after inspector
	var lastQuery string
	var lastResults *api.DockerHubSearchResponse
	var lastPage int
	// Track if this is the first iteration (for initial query)
	firstIteration := true

	for {
		model := NewDockerHubSearchModel(logger, database)

		// Pre-fill query if provided on first iteration (e.g., from DockerHub profile redirect)
		if firstIteration && initialQuery != "" {
			model.textInput.SetValue(initialQuery)
			model.query = initialQuery
			model.inputMode = false    // Don't show input mode, show search results
			model.searching = true     // Set searching state
			model.pendingSearch = true // Trigger search on Init
			firstIteration = false
		} else if lastQuery != "" && lastResults != nil {
			// Restore previous search state if we have it (after inspector returns)
			model.query = lastQuery
			model.results = lastResults
			model.page = lastPage
			model.inputMode = false // Start in table mode with results
			model.updateTable()     // Rebuild table rows from restored results
		}
		firstIteration = false

		p := tea.NewProgram(model, tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		m, ok := finalModel.(DockerHubSearchModel)
		if !ok {
			return nil
		}

		// Check if user wants to return to main
		if m.ShouldReturnToMain() {
			return nil
		}

		// Check if user wants to inspect an image
		if m.ShouldLaunchInspector() && m.SelectedImage() != "" {
			// Save current search state before launching inspector
			lastQuery = m.query
			lastResults = m.results
			lastPage = m.page

			// Prompt for tag
			tag, err := PromptForTag(m.SelectedImage())
			if err != nil {
				// User cancelled, go back to search with preserved state
				continue
			}

			imageRef := m.SelectedImage() + ":" + tag

			// Run the layer inspector
			if err := RunLayerInspectorWithDB(imageRef, database); err != nil {
				PrintError(fmt.Sprintf("Layer inspector error: %v", err))
			}

			// After inspector exits, loop back to search with preserved state
			continue
		}

		return nil
	}
}
