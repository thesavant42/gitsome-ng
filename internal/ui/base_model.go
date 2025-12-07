package ui

// base_model.go provides common TUI functionality for Bubble Tea models.
// Embed these helpers in models to reduce boilerplate for Init, Update, and View patterns.

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// BaseTableModel - Embed in table-based models
// =============================================================================

// BaseTableModel provides common table TUI functionality.
// Embed this in models to get standard Init, WindowSizeMsg handling,
// and two-box View rendering for free.
//
// Usage:
//
//	type myModel struct {
//	    BaseTableModel  // Embedded
//	    customField string
//	}
type BaseTableModel struct {
	Table    table.Model
	Layout   Layout
	Quitting bool
	Selected int // -1 = no selection
}

// NewBaseTableModel creates a BaseTableModel with default layout.
func NewBaseTableModel() BaseTableModel {
	return BaseTableModel{
		Layout:   DefaultLayout(),
		Selected: -1,
	}
}

// =============================================================================
// Table Initialization Helpers
// =============================================================================

// InitTable creates and configures a table with proper styling and dimensions.
// Use this instead of manually calling table.New() to ensure consistent setup.
//
// Example:
//
//	columns := CalculateColumns(mySpecs, layout.TableWidth)
//	rows := []table.Row{{"a", "b"}, {"c", "d"}}
//	m.Table = InitTable(columns, rows, layout)
func InitTable(columns []table.Column, rows []table.Row, layout Layout) table.Model {
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	// Apply standard table styles for consistent look and proper selection behavior
	ApplyTableStyles(&t)

	// Ensure cursor starts at the top for proper viewport positioning
	t.GotoTop()

	return t
}

// UpdateTableDimensions updates table height after a window resize.
// Call this in your updateTableSize() method after HandleWindowResize().
func (m *BaseTableModel) UpdateTableDimensions() {
	m.Table.SetHeight(m.Layout.TableHeight)
}

// =============================================================================
// Standard Init/Update Helpers
// =============================================================================

// StandardInit returns the standard Init command for table models.
// Call this from your model's Init() method.
func StandardInit() tea.Cmd {
	return tea.WindowSize()
}

// HandleWindowResize updates layout dimensions.
// Call this in your Update() for tea.WindowSizeMsg, then call your model-specific
// updateTableSize() method.
//
// Example:
//
//	case tea.WindowSizeMsg:
//	    m.HandleWindowResize(msg.Width, msg.Height)
//	    m.updateTableSize()
//	    return m, nil
func (m *BaseTableModel) HandleWindowResize(width, height int) {
	m.Layout = NewLayout(width, height)
}

// =============================================================================
// Key Handling Helpers
// =============================================================================

// HandleQuitKeys returns true and Quit cmd for q/esc/ctrl+c keys.
// Use in your Update() to standardize quit behavior.
//
// Example:
//
//	case tea.KeyMsg:
//	    if quit, cmd := HandleQuitKeys(msg.String()); quit {
//	        m.Quitting = true
//	        return m, cmd
//	    }
func HandleQuitKeys(key string) (bool, tea.Cmd) {
	switch key {
	case "q", "esc", "ctrl+c":
		return true, tea.Quit
	}
	return false, nil
}

// HandleQuitKeysNoEsc returns true and Quit cmd for q/ctrl+c keys (not esc).
// Use when esc has special meaning (e.g., cancel input mode).
func HandleQuitKeysNoEsc(key string) (bool, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return true, tea.Quit
	}
	return false, nil
}

// HandleSelectKey returns cursor position and true if enter pressed.
// Use for table selection behavior.
//
// Example:
//
//	if sel, ok := HandleSelectKey(msg.String(), m.Table.Cursor()); ok {
//	    m.Selected = sel
//	    m.Quitting = true
//	    return m, tea.Quit
//	}
func HandleSelectKey(key string, cursor int) (int, bool) {
	if key == "enter" {
		return cursor, true
	}
	return -1, false
}

// HandleNavigationKeys handles standard up/down/j/k navigation.
// Returns new cursor position (clamped to valid range).
func HandleNavigationKeys(key string, cursor, maxItems int) int {
	switch key {
	case "up", "k":
		if cursor > 0 {
			return cursor - 1
		}
	case "down", "j":
		if cursor < maxItems-1 {
			return cursor + 1
		}
	}
	return cursor
}

// =============================================================================
// Selection State Helpers
// =============================================================================

// HasSelection returns true if a selection was made (Selected >= 0).
func (m BaseTableModel) HasSelection() bool {
	return m.Selected >= 0
}

// GetSelectedRow returns the selected row data, or nil if no selection.
func (m BaseTableModel) GetSelectedRow() table.Row {
	if m.Selected >= 0 && m.Selected < len(m.Table.Rows()) {
		return m.Table.Rows()[m.Selected]
	}
	return nil
}
