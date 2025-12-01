package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
)

// DockerHubSearchModel is the TUI model for Docker Hub search
type DockerHubSearchModel struct {
	client          *api.DockerHubClient
	logger          *log.Logger
	database        *db.DB
	layout          Layout
	table           table.Model
	textInput       textinput.Model
	results         *api.DockerHubSearchResponse
	query           string
	page            int
	searching       bool
	inputMode       bool
	err             error
	quitting        bool
	returnToMain    bool
	launchInspector bool           // true when user wants to inspect an image
	selectedImage   string         // image name to inspect
	cachedImages    map[string]int // image name -> layer count (from DB)
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
	ti.Width = 50

	// Calculate layout
	layout := DefaultLayout()

	// Create table with initial columns
	columns := []table.Column{
		{Title: "Name"},
		{Title: "Publisher"},
		{Title: "Stars"},
		{Title: "Pulls"},
		{Title: "Badge"},
		{Title: "Description"},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(TableHeight),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(lipgloss.NoColor{}).
		Bold(true)
	t.SetStyles(s)

	return DockerHubSearchModel{
		client:       api.NewDockerHubClient(logger),
		logger:       logger,
		database:     database,
		layout:       layout,
		table:        t,
		textInput:    ti,
		page:         1,
		inputMode:    true,
		cachedImages: make(map[string]int),
	}
}

// Init implements tea.Model
func (m DockerHubSearchModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m DockerHubSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.table.SetHeight(TableHeight)
		if m.results != nil {
			m.updateTable()
		}
		return m, nil

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

	var b strings.Builder
	b.WriteString("\n")

	// Build content inside border
	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Docker Hub Search"))
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

		if m.searching {
			contentBuilder.WriteString(HintStyle.Render(" Searching..."))
		} else if m.err != nil {
			contentBuilder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf(" Error: %v", m.err)))
		} else {
			// Render table with selection highlighting
			tableView := m.renderTableWithSelection()
			contentBuilder.WriteString(tableView)
		}
	}

	// Wrap in bordered box with padding like main TUI
	borderedContent := BorderStyle.
		Padding(1, 0).
		Render(contentBuilder.String())
	b.WriteString(borderedContent)
	b.WriteString("\n")

	// Help text OUTSIDE border (like main TUI footer)
	if m.inputMode {
		b.WriteString(" " + HintStyle.Render("Enter: search | Esc: back to main"))
	} else {
		b.WriteString(" " + HintStyle.Render("Enter: inspect layers | /: new search | n/->: next | p/<-: prev | up/down: navigate | Esc: back"))
	}

	return b.String()
}

// doSearch performs the search asynchronously
func (m DockerHubSearchModel) doSearch() tea.Cmd {
	return func() tea.Msg {
		results, err := m.client.Search(m.query, m.page)
		return DockerHubSearchMsg{Results: results, Err: err}
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

	rows := make([]table.Row, len(m.results.Results))
	for i, r := range m.results.Results {
		desc := r.ShortDescription

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

		rows[i] = table.Row{
			name,
			r.Publisher,
			fmt.Sprintf("%d", r.StarCount),
			r.PullCount,
			badge,
			desc,
		}
	}

	// Calculate column widths from content (except Description)
	headers := []string{"Name", "Publisher", "Stars", "Pulls", "Badge", "Description"}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i := 0; i < 5; i++ { // Skip Description (index 5)
			if len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}

	// Description gets remaining space
	usedWidth := widths[0] + widths[1] + widths[2] + widths[3] + widths[4]
	separators := 7 // table borders and column separators
	widths[5] = m.layout.ViewportWidth - usedWidth - separators
	if widths[5] < len(headers[5]) {
		widths[5] = len(headers[5])
	}

	columns := []table.Column{
		{Title: "Name", Width: widths[0]},
		{Title: "Publisher", Width: widths[1]},
		{Title: "Stars", Width: widths[2]},
		{Title: "Pulls", Width: widths[3]},
		{Title: "Badge", Width: widths[4]},
		{Title: "Description", Width: widths[5]},
	}
	m.table.SetColumns(columns)
	m.table.SetRows(rows)
}

// renderTableWithSelection renders the table with custom selection highlighting
func (m DockerHubSearchModel) renderTableWithSelection() string {
	if m.results == nil || len(m.results.Results) == 0 {
		return HintStyle.Render("  No results found")
	}

	// Get table view and split into lines
	tableView := m.table.View()
	lines := strings.Split(tableView, "\n")

	cursor := m.table.Cursor()
	headerLines := 2 // Header + border

	// Calculate the width for the selection bar - match table content width
	// Use layout width minus border padding (2 chars for border)
	selectionWidth := m.layout.ViewportWidth - 4
	if selectionWidth < 40 {
		selectionWidth = 40
	}

	var result []string
	for i, line := range lines {
		dataRowIndex := i - headerLines
		if dataRowIndex >= 0 && dataRowIndex == cursor {
			// Apply selection style with constrained width to match table
			result = append(result, SelectedStyle.Width(selectionWidth).Render(line))
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
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
func RunDockerHubSearch(logger *log.Logger, database *db.DB) error {
	// Keep track of the last search state to restore after inspector
	var lastQuery string
	var lastResults *api.DockerHubSearchResponse
	var lastPage int

	for {
		model := NewDockerHubSearchModel(logger, database)

		// Restore previous search state if we have it
		if lastQuery != "" && lastResults != nil {
			model.query = lastQuery
			model.results = lastResults
			model.page = lastPage
			model.inputMode = false // Start in table mode with results
			model.updateTable()     // Rebuild table rows from restored results
		}

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
