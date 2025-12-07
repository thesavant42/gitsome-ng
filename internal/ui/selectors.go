package ui

// selectors.go provides generic single-column selector models.
// Use these instead of creating separate platformSelectorModel, tagSelectorModel, etc.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// Generic Selector Model
// =============================================================================

// SelectorConfig defines configuration for a generic selector.
type SelectorConfig struct {
	Title    string   // Main title displayed at top
	Subtitle string   // Optional subtitle (e.g., "5 items available")
	HelpText string   // Help text for footer (e.g., "↑/↓: navigate | Enter: select")
	Items    []string // Display labels for each option
	Values   []string // Optional: actual values (if different from display labels)
}

// SelectorModel is a generic single-column table selector.
// Replaces platformSelectorModel, tagSelectorModel, layerActionSelectorModel, etc.
type SelectorModel struct {
	table    table.Model
	config   SelectorConfig
	layout   Layout
	selected int // Index of selected item, -1 if cancelled
	quitting bool
}

// NewSelectorModel creates a generic selector with the given configuration.
func NewSelectorModel(cfg SelectorConfig) SelectorModel {
	layout := DefaultLayout()

	// Build table rows from items
	rows := make([]table.Row, len(cfg.Items))
	for i, item := range cfg.Items {
		rows[i] = table.Row{item}
	}

	// Single column using full table width
	columns := []table.Column{
		{Title: cfg.Title, Width: layout.TableWidth},
	}

	// Initialize table with standard setup
	t := InitTable(columns, rows, layout)

	// Default help text if not provided
	helpText := cfg.HelpText
	if helpText == "" {
		helpText = "↑/↓: navigate | Enter: select | Esc: cancel"
	}
	cfg.HelpText = helpText

	return SelectorModel{
		table:    t,
		config:   cfg,
		layout:   layout,
		selected: -1,
	}
}

func (m SelectorModel) Init() tea.Cmd {
	return StandardInit()
}

func (m SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.selected = -1
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.selected = m.table.Cursor()
			m.quitting = true
			return m, tea.Quit
		}
	}

	// Let table handle navigation
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *SelectorModel) updateTableSize() {
	columns := []table.Column{
		{Title: m.config.Title, Width: m.layout.TableWidth},
	}
	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func (m SelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var content strings.Builder

	// Header with title and optional subtitle
	if m.config.Subtitle != "" {
		content.WriteString(ViewHeaderWithSubtitle(m.config.Title, m.config.Subtitle, m.layout.InnerWidth))
	} else {
		content.WriteString(ViewHeader(m.config.Title, m.layout.InnerWidth))
	}

	// Table with full-width selection highlighting
	content.WriteString(RenderTableWithSelection(m.table, m.layout))

	// Use TwoBoxView for consistent two-box layout
	return TwoBoxView(content.String(), m.config.HelpText, m.layout)
}

// Selected returns the index of the selected item, or -1 if cancelled.
func (m SelectorModel) Selected() int {
	return m.selected
}

// SelectedValue returns the value of the selected item.
// If Values were provided in config, returns the corresponding value.
// Otherwise returns the display label.
func (m SelectorModel) SelectedValue() string {
	if m.selected < 0 || m.selected >= len(m.config.Items) {
		return ""
	}
	if len(m.config.Values) > m.selected {
		return m.config.Values[m.selected]
	}
	return m.config.Items[m.selected]
}

// =============================================================================
// Convenience Functions for Common Selectors
// =============================================================================

// RunSelector runs a selector TUI and returns the selected index.
// Returns -1 if the user cancelled.
func RunSelector(cfg SelectorConfig) (int, error) {
	model := NewSelectorModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return -1, fmt.Errorf("selector error: %w", err)
	}
	return finalModel.(SelectorModel).Selected(), nil
}

// RunSelectorWithValue runs a selector TUI and returns the selected value.
// Returns empty string if the user cancelled.
func RunSelectorWithValue(cfg SelectorConfig) (string, error) {
	model := NewSelectorModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("selector error: %w", err)
	}
	return finalModel.(SelectorModel).SelectedValue(), nil
}
