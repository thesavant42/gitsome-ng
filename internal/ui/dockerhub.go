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
)

// DockerHubSearchModel is the TUI model for Docker Hub search
type DockerHubSearchModel struct {
	client     *api.DockerHubClient
	logger     *log.Logger
	layout     Layout
	table      table.Model
	textInput  textinput.Model
	results    *api.DockerHubSearchResponse
	query      string
	page       int
	searching  bool
	inputMode  bool
	err        error
	quitting   bool
	returnToMain bool
}

// DockerHubSearchMsg is sent when search results are ready
type DockerHubSearchMsg struct {
	Results *api.DockerHubSearchResponse
	Err     error
}

// NewDockerHubSearchModel creates a new Docker Hub search TUI
func NewDockerHubSearchModel(logger *log.Logger) DockerHubSearchModel {
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
		{Title: "Name", Width: 35},
		{Title: "Publisher", Width: 15},
		{Title: "Stars", Width: 8},
		{Title: "Pulls", Width: 10},
		{Title: "Badge", Width: 12},
		{Title: "Description", Width: 40},
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
		client:    api.NewDockerHubClient(logger),
		logger:    logger,
		layout:    layout,
		table:     t,
		textInput: ti,
		page:      1,
		inputMode: true,
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
		m.layout = NewLayout(msg.Width)
		m.table.SetHeight(TableHeight)
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
			// Could open Docker Hub URL in browser
			if m.results != nil && len(m.results.Results) > 0 {
				cursor := m.table.Cursor()
				if cursor >= 0 && cursor < len(m.results.Results) {
					// For now, just show selected - could open browser later
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
		contentBuilder.WriteString("\n\n")
		contentBuilder.WriteString(HintStyle.Render(" Enter: search | Esc: back to main"))
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

		contentBuilder.WriteString("\n\n")
		contentBuilder.WriteString(HintStyle.Render(" /: new search | n/->: next page | p/<-: prev page | j/k: navigate | Esc: back"))
	}

	// Wrap in bordered box
	borderedContent := BorderStyle.
		Width(m.layout.ViewportWidth).
		Padding(1, 0).
		Render(contentBuilder.String())
	b.WriteString(borderedContent)

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

	rows := make([]table.Row, len(m.results.Results))
	for i, r := range m.results.Results {
		// Truncate description if too long
		desc := r.ShortDescription
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}

		badge := r.Badge
		if badge == "verified_publisher" {
			badge = "Verified"
		} else if badge == "official" {
			badge = "Official"
		} else {
			badge = ""
		}

		rows[i] = table.Row{
			r.Name,
			r.Publisher,
			fmt.Sprintf("%d", r.StarCount),
			r.PullCount,
			badge,
			desc,
		}
	}

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

	var result []string
	for i, line := range lines {
		dataRowIndex := i - headerLines
		if dataRowIndex >= 0 && dataRowIndex == cursor {
			result = append(result, SelectedStyle.Width(m.layout.InnerWidth).Render(line))
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

// RunDockerHubSearch starts the Docker Hub search TUI
func RunDockerHubSearch(logger *log.Logger) error {
	model := NewDockerHubSearchModel(logger)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// Check if user wants to return to main
	if m, ok := finalModel.(DockerHubSearchModel); ok && m.ShouldReturnToMain() {
		return nil
	}

	return nil
}

