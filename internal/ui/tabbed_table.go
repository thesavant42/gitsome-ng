package ui

// tabbed_table.go provides a generic multi-page tabbed table viewer.
// Use this for ANY view that needs multiple pages of data with Tab/←/→ switching.
//
// USAGE EXAMPLES:
//   - Layer inspector: Layers (selectable) + Build Steps (read-only)
//   - User detail: Profile + Repos (selectable) + Gists (selectable)
//   - Image browser: Tags + Manifests + Config
//
// This component follows the established patterns:
//   - Uses TwoBoxView for consistent two-box layout (main + footer)
//   - Uses RenderTableWithSelection for full-width selection highlighting
//   - Uses InitTable/ApplyTableStyles for consistent table styling
//   - Uses ColumnSpec/CalculateColumns for flexible column widths

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// Configuration Types
// =============================================================================

// TabbedTablePage defines a single page/tab of data.
type TabbedTablePage struct {
	Name     string       // Tab label (e.g., "Layers", "Build Steps")
	Columns  []ColumnSpec // Column specifications (use columns.go helpers)
	Rows     []table.Row  // Row data
	ReadOnly bool         // If true, Enter doesn't select; just scrolling allowed
	HelpText string       // Optional per-page help text (overrides default)
}

// TabbedTableConfig defines the complete configuration for a tabbed table.
type TabbedTableConfig struct {
	Title    string            // Main title (e.g., "Layer Inspector: nginx:latest")
	Subtitle string            // Optional subtitle
	Pages    []TabbedTablePage // Pages to display (at least 1 required)
	HelpText string            // Default footer help text (pages can override)
}

// TabbedTableResult contains the result after the TUI exits.
type TabbedTableResult struct {
	SelectedPage int  // Which page was active when selection was made
	SelectedRow  int  // Index of selected row (-1 if cancelled or read-only page)
	Cancelled    bool // True if user pressed q/Esc
}

// =============================================================================
// TabbedTableModel - The Bubble Tea Model
// =============================================================================

// TabbedTableModel is a generic multi-page table viewer.
// Embed configuration, manage per-page tables, handle tab switching.
type TabbedTableModel struct {
	config      TabbedTableConfig
	tables      []table.Model // One table per page
	currentPage int
	layout      Layout
	result      TabbedTableResult
	quitting    bool
}

// NewTabbedTableModel creates a new tabbed table viewer.
func NewTabbedTableModel(cfg TabbedTableConfig) TabbedTableModel {
	layout := DefaultLayout()

	// Validate config
	if len(cfg.Pages) == 0 {
		// Add a placeholder page to prevent crashes
		cfg.Pages = []TabbedTablePage{{
			Name:    "Empty",
			Columns: []ColumnSpec{{Title: "No Data", FlexRatio: 100}},
			Rows:    []table.Row{{"No pages configured"}},
		}}
	}

	// Default help text
	if cfg.HelpText == "" {
		if len(cfg.Pages) > 1 {
			cfg.HelpText = "↑/↓: navigate | Tab/←/→: switch page | Enter: select | q/Esc: back"
		} else {
			cfg.HelpText = "↑/↓: navigate | Enter: select | q/Esc: back"
		}
	}

	// Create a table for each page
	tables := make([]table.Model, len(cfg.Pages))
	for i, page := range cfg.Pages {
		// Calculate columns for this page
		columns := CalculateColumns(page.Columns, layout.TableWidth)

		// Create table with standard initialization
		t := table.New(
			table.WithColumns(columns),
			table.WithRows(page.Rows),
			table.WithFocused(i == 0), // Only first page is focused initially
			table.WithHeight(layout.TabbedTableHeight()),
		)
		ApplyTableStyles(&t)
		t.GotoTop()
		tables[i] = t
	}

	return TabbedTableModel{
		config:      cfg,
		tables:      tables,
		currentPage: 0,
		layout:      layout,
		result: TabbedTableResult{
			SelectedPage: -1,
			SelectedRow:  -1,
			Cancelled:    false,
		},
	}
}

// =============================================================================
// Bubble Tea Interface
// =============================================================================

func (m TabbedTableModel) Init() tea.Cmd {
	return StandardInit()
}

func (m TabbedTableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateAllTableSizes()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Let the current table handle other messages (scroll, etc.)
	var cmd tea.Cmd
	if m.currentPage >= 0 && m.currentPage < len(m.tables) {
		m.tables[m.currentPage], cmd = m.tables[m.currentPage].Update(msg)
	}
	return m, cmd
}

func (m TabbedTableModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys (work on all pages)
	switch key {
	case "q", "esc":
		m.result.Cancelled = true
		m.quitting = true
		return m, tea.Quit

	case "tab", "right", "l":
		// Switch to next page
		if len(m.config.Pages) > 1 {
			m.switchPage((m.currentPage + 1) % len(m.config.Pages))
		}
		return m, nil

	case "left", "h":
		// Switch to previous page (only if multiple pages)
		if len(m.config.Pages) > 1 {
			m.switchPage((m.currentPage + len(m.config.Pages) - 1) % len(m.config.Pages))
		}
		return m, nil
	}

	// Page-specific keys
	currentPage := m.config.Pages[m.currentPage]

	switch key {
	case "enter":
		// Only process enter on non-read-only pages
		if !currentPage.ReadOnly {
			cursor := m.tables[m.currentPage].Cursor()
			if cursor >= 0 && cursor < len(currentPage.Rows) {
				m.result.SelectedPage = m.currentPage
				m.result.SelectedRow = cursor
				m.quitting = true
				return m, tea.Quit
			}
		}
		return m, nil

	case "up", "k":
		m.tables[m.currentPage].MoveUp(1)
		return m, nil

	case "down", "j":
		m.tables[m.currentPage].MoveDown(1)
		return m, nil

	case "home", "g":
		m.tables[m.currentPage].GotoTop()
		return m, nil

	case "end", "G":
		m.tables[m.currentPage].GotoBottom()
		return m, nil

	case "pgup", "ctrl+u":
		// Move up by half page
		height := m.tables[m.currentPage].Height()
		for i := 0; i < height/2; i++ {
			m.tables[m.currentPage].MoveUp(1)
		}
		return m, nil

	case "pgdown", "ctrl+d":
		// Move down by half page
		height := m.tables[m.currentPage].Height()
		for i := 0; i < height/2; i++ {
			m.tables[m.currentPage].MoveDown(1)
		}
		return m, nil
	}

	return m, nil
}

func (m *TabbedTableModel) switchPage(newPage int) {
	if newPage < 0 || newPage >= len(m.config.Pages) {
		return
	}

	// Blur old table, focus new
	m.tables[m.currentPage].Blur()
	m.currentPage = newPage
	m.tables[m.currentPage].Focus()
	m.tables[m.currentPage].GotoTop()
}

func (m *TabbedTableModel) updateAllTableSizes() {
	for i, page := range m.config.Pages {
		columns := CalculateColumns(page.Columns, m.layout.TableWidth)
		m.tables[i].SetColumns(columns)
		m.tables[i].SetHeight(m.layout.TabbedTableHeight())
	}
} // =============================================================================
// View Rendering
// =============================================================================

func (m TabbedTableModel) View() string {
	if m.quitting {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(RenderTitle(m.config.Title))
	content.WriteString("\n")

	// Tab indicator (only if multiple pages)
	if len(m.config.Pages) > 1 {
		content.WriteString(m.renderTabIndicator())
		content.WriteString("\n")
	}

	// Divider
	content.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	content.WriteString("\n\n")

	// Subtitle if present
	if m.config.Subtitle != "" {
		content.WriteString(RenderDim(m.config.Subtitle))
		content.WriteString("\n\n")
	}

	// Table with full-width selection
	content.WriteString(RenderTableWithSelection(m.tables[m.currentPage], m.layout))

	// Get help text (page-specific or default)
	currentPage := m.config.Pages[m.currentPage]
	helpText := currentPage.HelpText
	if helpText == "" {
		helpText = m.config.HelpText
	}

	// Use TwoBoxView for consistent layout
	return TwoBoxView(content.String(), helpText, m.layout)
}

func (m TabbedTableModel) renderTabIndicator() string {
	var parts []string
	for i, page := range m.config.Pages {
		if i == m.currentPage {
			parts = append(parts, RenderTabActive(page.Name))
		} else {
			parts = append(parts, RenderTabInactive(page.Name))
		}
	}
	indicator := strings.Join(parts, " ")

	// Add navigation hint
	if len(m.config.Pages) > 1 {
		indicator += "  " + RenderDim("(Tab/←/→)")
	}

	return indicator
}

// Result returns the selection result after the TUI exits.
func (m TabbedTableModel) Result() TabbedTableResult {
	return m.result
}

// =============================================================================
// Convenience Functions
// =============================================================================

// RunTabbedTable runs a tabbed table TUI and returns the result.
func RunTabbedTable(cfg TabbedTableConfig) (TabbedTableResult, error) {
	model := NewTabbedTableModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return TabbedTableResult{Cancelled: true}, fmt.Errorf("tabbed table error: %w", err)
	}
	return finalModel.(TabbedTableModel).Result(), nil
}

// =============================================================================
// Builder Helpers - Fluent API for common patterns
// =============================================================================

// TabbedTableBuilder provides a fluent API for building TabbedTableConfig.
type TabbedTableBuilder struct {
	config TabbedTableConfig
}

// NewTabbedTable starts building a new tabbed table configuration.
func NewTabbedTable(title string) *TabbedTableBuilder {
	return &TabbedTableBuilder{
		config: TabbedTableConfig{
			Title: title,
			Pages: []TabbedTablePage{},
		},
	}
}

// WithSubtitle sets the subtitle.
func (b *TabbedTableBuilder) WithSubtitle(subtitle string) *TabbedTableBuilder {
	b.config.Subtitle = subtitle
	return b
}

// WithHelpText sets the default help text.
func (b *TabbedTableBuilder) WithHelpText(helpText string) *TabbedTableBuilder {
	b.config.HelpText = helpText
	return b
}

// AddPage adds a page to the tabbed table.
func (b *TabbedTableBuilder) AddPage(name string, columns []ColumnSpec, rows []table.Row) *TabbedTableBuilder {
	b.config.Pages = append(b.config.Pages, TabbedTablePage{
		Name:     name,
		Columns:  columns,
		Rows:     rows,
		ReadOnly: false,
	})
	return b
}

// AddReadOnlyPage adds a read-only page (scrollable but not selectable).
func (b *TabbedTableBuilder) AddReadOnlyPage(name string, columns []ColumnSpec, rows []table.Row) *TabbedTableBuilder {
	b.config.Pages = append(b.config.Pages, TabbedTablePage{
		Name:     name,
		Columns:  columns,
		Rows:     rows,
		ReadOnly: true,
	})
	return b
}

// Build returns the completed configuration.
func (b *TabbedTableBuilder) Build() TabbedTableConfig {
	return b.config
}

// Run builds and runs the tabbed table, returning the result.
func (b *TabbedTableBuilder) Run() (TabbedTableResult, error) {
	return RunTabbedTable(b.Build())
}

// =============================================================================
// Pre-defined Column Specs for Common Use Cases
// =============================================================================

// LayerSelectorColumns returns column specs for the layer selector page.
func LayerSelectorColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Layer", FixedWidth: 8},
		{Title: "Digest", FlexRatio: 100, MinWidth: 20},
		{Title: "Size", FixedWidth: 12},
	}
}

// BuildStepsColumns returns column specs for the build steps page.
func BuildStepsColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Build Steps (Dockerfile History)", FlexRatio: 100},
	}
}

// CachedLayerColumns returns column specs for cached layer display.
func CachedLayerColumns() []ColumnSpec {
	return []ColumnSpec{
		{Title: "Layer", FixedWidth: 8},
		{Title: "Digest", FlexRatio: 60, MinWidth: 12},
		{Title: "Size", FixedWidth: 12},
		{Title: "Entries", FixedWidth: 10},
	}
}
