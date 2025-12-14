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
	filteredResults   []api.DockerHubSearchResult // results after applying type filter
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
	filterType        string         // current filter: "all", "image", "user", "model"
}

// Filter type constants
const (
	FilterAll   = "all"
	FilterImage = "image"
	FilterUser  = "user"
	FilterModel = "model"
)

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
		filterType:   FilterAll,
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

		case "f":
			// Cycle through filter types: all -> image -> user -> model -> all
			switch m.filterType {
			case FilterAll:
				m.filterType = FilterImage
			case FilterImage:
				m.filterType = FilterUser
			case FilterUser:
				m.filterType = FilterModel
			case FilterModel:
				m.filterType = FilterAll
			}
			m.updateTable()
			return m, nil

		case "1":
			// Filter: All
			m.filterType = FilterAll
			m.updateTable()
			return m, nil

		case "2":
			// Filter: Images only
			m.filterType = FilterImage
			m.updateTable()
			return m, nil

		case "3":
			// Filter: Users only
			m.filterType = FilterUser
			m.updateTable()
			return m, nil

		case "4":
			// Filter: Models only
			m.filterType = FilterModel
			m.updateTable()
			return m, nil

		case "enter":
			// Launch layer inspector for selected image
			fmt.Println("DEBUG: Enter pressed in table mode")
			if len(m.filteredResults) > 0 {
				cursor := m.table.Cursor()
				fmt.Printf("DEBUG: cursor=%d, filteredResults=%d\n", cursor, len(m.filteredResults))
				if cursor >= 0 && cursor < len(m.filteredResults) {
					selected := m.filteredResults[cursor]
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

	// Input mode - show search input
	if m.inputMode {
		return NewPageView(m.layout).
			Title("Docker Hub Search").
			Divider().
			Spacing(2).
			Text(" Search: " + m.textInput.View()).
			Help("Enter: search | Esc: back to main").
			Build()
	}

	// Table mode - build query info
	queryInfo := fmt.Sprintf(" Query: %s", m.query)
	if m.results != nil {
		totalPages := (m.results.Total + 24) / 25 // 25 results per page, round up
		queryInfo += fmt.Sprintf("  |  Page %d of %d  |  Total: %d results", m.page, totalPages, m.results.Total)
	}

	// Add filter status
	filterLabel := ""
	switch m.filterType {
	case FilterAll:
		filterLabel = "[All]"
	case FilterImage:
		filterLabel = "[Images]"
	case FilterUser:
		filterLabel = "[Users]"
	case FilterModel:
		filterLabel = "[Models]"
	}
	if m.filterType != FilterAll && m.results != nil {
		queryInfo += fmt.Sprintf("  |  Filter: %s (%d shown)", filterLabel, len(m.filteredResults))
	} else if m.filterType != FilterAll {
		queryInfo += fmt.Sprintf("  |  Filter: %s", filterLabel)
	}

	// Build view with PageViewBuilder
	builder := NewPageView(m.layout).
		Title("Docker Hub Search").
		Divider().
		Spacing(2).
		QueryInfo(queryInfo)

	// Add content based on state
	if m.searching {
		builder.CustomContent(m.spinner.View() + " " + HintStyle.Render("Searching..."))
	} else if m.err != nil {
		builder.Error(m.err)
	} else {
		builder.Table(m.table)
	}

	return builder.Help("Enter: inspect | /: search | n: next | p: prev | f: filter (1-4) | Esc: back").
		Build()
}

// doSearch performs the search asynchronously
func (m DockerHubSearchModel) doSearch() tea.Cmd {
	return func() tea.Msg {
		results, err := m.client.Search(m.query, m.page)
		return DockerHubSearchMsg{Results: results, Err: err}
	}
}

// calculateDockerHubColumns uses the centralized CalculateColumns with DockerHubColumns spec.
func calculateDockerHubColumns(totalW int) []table.Column {
	return CalculateColumns(DockerHubColumns(), totalW)
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

	// Apply type filter to results
	m.filteredResults = nil
	for _, r := range m.results.Results {
		switch m.filterType {
		case FilterAll:
			m.filteredResults = append(m.filteredResults, r)
		case FilterImage:
			// Docker Hub uses "image" type for containers
			if r.Type == "image" {
				m.filteredResults = append(m.filteredResults, r)
			}
		case FilterUser:
			// Docker Hub may use "user" or we check if name contains only the publisher (no slash)
			if r.Type == "user" || (r.Type == "" && !strings.Contains(r.Name, "/")) {
				m.filteredResults = append(m.filteredResults, r)
			}
		case FilterModel:
			// Docker Hub AI models have "model" type
			if r.Type == "model" {
				m.filteredResults = append(m.filteredResults, r)
			}
		}
	}

	// Get column widths from central calculation function (uses TableWidth for bubbles overhead)
	columns := calculateDockerHubColumns(m.layout.TableWidth)

	// Extract widths for truncation
	nameW := columns[0].Width
	publisherW := columns[1].Width
	starsW := columns[2].Width
	cachedW := columns[3].Width
	descW := columns[4].Width

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

	rows := make([]table.Row, len(m.filteredResults))
	for i, r := range m.filteredResults {
		desc := truncate(r.ShortDescription, descW)

		// Check if we have cached layer data for this image
		var cachedStr string
		if layerCount, ok := m.cachedImages[r.Name]; ok {
			cachedStr = fmt.Sprintf("[%d] Y", layerCount)
		}

		name := truncate(r.Name, nameW)
		publisher := truncate(r.Publisher, publisherW)
		stars := truncate(fmt.Sprintf("%d", r.StarCount), starsW)
		cachedStr = truncate(cachedStr, cachedW)

		rows[i] = table.Row{
			name,
			publisher,
			stars,
			cachedStr,
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

// PromptForDockerHubPath prompts the user to enter a Docker Hub repository path
// Uses the generic RunInput for consistent UI
func PromptForDockerHubPath() (string, error) {
	value, cancelled, err := RunInput(InputConfig{
		Title:       "Browse DockerHub Repository",
		Subtitle:    "Examples: nginx, library/alpine, myuser/myapp",
		Placeholder: "user/container (e.g., nginx, library/alpine, myuser/myapp)",
		HelpText:    "Enter: confirm | Esc: cancel",
		Validator: func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("path cannot be empty")
			}
			return nil
		},
	})
	if err != nil {
		return "", fmt.Errorf("prompt error: %w", err)
	}
	if cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return value, nil
}

// RunBrowseDockerHubRepo prompts for a Docker Hub path and launches the layer inspector
// If the repository doesn't exist or has no tags, displays a 404 error
func RunBrowseDockerHubRepo(logger *log.Logger, database *db.DB) error {
	// Prompt for the Docker Hub path
	path, err := PromptForDockerHubPath()
	if err != nil {
		if strings.Contains(err.Error(), "cancelled") {
			return nil // User cancelled, return to menu
		}
		return err
	}

	// Normalize path - if no slash, treat as official image (library/name)
	imageName := path
	if !strings.Contains(path, "/") {
		imageName = "library/" + path
	}

	// Try to fetch tags to validate the repository exists
	client := api.NewRegistryClient()
	tags, err := client.ListTags(imageName)

	if err != nil || len(tags) == 0 {
		// Repository not found or has no tags - show 404 error
		show404Error(imageName)
		return nil
	}

	// Repository exists - proceed with tag selection and inspection
	tag, err := PromptForTag(imageName)
	if err != nil {
		if strings.Contains(err.Error(), "cancelled") || strings.Contains(err.Error(), "no tag selected") {
			return nil
		}
		return err
	}

	imageRef := imageName + ":" + tag

	// Run the layer inspector
	if err := RunLayerInspectorWithDB(imageRef, database); err != nil {
		PrintError(fmt.Sprintf("Layer inspector error: %v", err))
	}

	return nil
}

// show404Error displays a 404 error message for a Docker Hub repository
// Uses RunSelector with a single "Return" option for consistent UI
func show404Error(imageName string) {
	RunSelector(SelectorConfig{
		Title:    "DockerHub Repository Not Found",
		Subtitle: fmt.Sprintf("404 - Repository '%s' not found\n\nThe repository may not exist, may be private, or has no available tags.\nPlease verify the path and try again.", imageName),
		Items:    []string{"Press Enter to return"},
		HelpText: "Enter: return",
	})
}
