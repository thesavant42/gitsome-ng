package ui

// LIPGLOSS-FREE: This file uses centralized styles from styles.go
// All lipgloss usage has been moved to styles.go per the style guide.
// DO NOT add lipgloss import here.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	bubbleSpinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh/spinner"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
)

// =============================================================================
// Helper Functions
// =============================================================================

// sanitizeBuildStep normalizes whitespace in build steps to prevent layout issues.
// Replaces tabs and multiple consecutive spaces with a single space.
var multiSpaceRegex = regexp.MustCompile(`[\t ]+`)

func sanitizeBuildStep(step string) string {
	// Replace tabs and multiple spaces with single space
	result := multiSpaceRegex.ReplaceAllString(step, " ")
	// Trim leading/trailing whitespace
	return strings.TrimSpace(result)
}

// wrapBuildStep wraps a build step to fit within width
func wrapBuildStep(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	var result strings.Builder
	remaining := text
	for len(remaining) > 0 {
		if len(remaining) <= width {
			result.WriteString(remaining)
			break
		}
		// Find break point
		breakPoint := width
		for i := width; i > width/2; i-- {
			c := remaining[i-1]
			if c == ' ' || c == '/' || c == '-' || c == '_' {
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

// =============================================================================
// Filesystem Tree Structure
// =============================================================================

// fsNode represents a node in the filesystem tree
type fsNode struct {
	name     string
	size     int64
	isDir    bool
	children map[string]*fsNode
	parent   *fsNode
}

// buildFSTree constructs a filesystem tree from flat TarEntry slice
func buildFSTree(entries []api.TarEntry) *fsNode {
	root := &fsNode{
		name:     "/",
		isDir:    true,
		children: make(map[string]*fsNode),
	}

	for _, e := range entries {
		parts := strings.Split(strings.Trim(e.Name, "/"), "/")
		current := root
		for i, part := range parts {
			if part == "" {
				continue
			}
			if current.children[part] == nil {
				current.children[part] = &fsNode{
					name:     part,
					children: make(map[string]*fsNode),
					parent:   current,
				}
			}
			current = current.children[part]
			if i == len(parts)-1 {
				current.isDir = e.IsDir
				current.size = e.Size
			}
		}
	}

	return root
}

// getPath returns the full path to this node
func (n *fsNode) getPath() string {
	if n.parent == nil {
		return "/"
	}
	parentPath := n.parent.getPath()
	if parentPath == "/" {
		return "/" + n.name
	}
	return parentPath + "/" + n.name
}

// getSortedChildren returns children sorted: directories first, then alphabetically
func (n *fsNode) getSortedChildren() []*fsNode {
	children := make([]*fsNode, 0, len(n.children))
	for _, child := range n.children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].isDir != children[j].isDir {
			return children[i].isDir // dirs first
		}
		return children[i].name < children[j].name
	})
	return children
}

// =============================================================================
// Shared Table Rendering Helper
// =============================================================================

// renderTableWithFullWidthSelection renders a bubbles table with full-width selection highlight.
// The table's Selected style should use Background(lipgloss.NoColor{}) to prevent ANSI embedding,
// and this function applies the visible selection styling.
// This matches the pattern used in tui.go's renderBubblesTableWithFullWidth().
func renderTableWithFullWidthSelection(t table.Model, layout Layout) string {
	tableOutput := t.View()
	lines := strings.Split(tableOutput, "\n")
	var result []string

	cursor := t.Cursor()

	for i, line := range lines {
		// Header row - apply full width for consistent rendering
		if i == 0 {
			result = append(result, NormalStyle.Width(layout.InnerWidth).Render(line))
			continue
		}

		// Divider line (line 1) - use InnerWidth for full width
		if i == 1 {
			result = append(result, strings.Repeat("─", layout.InnerWidth))
			continue
		}

		// Data rows start at line 2, so dataRowIndex = i - 2
		dataRowIndex := i - 2

		// Apply full-width selection styling to the selected row
		// The table component handles scrolling internally, so we just need to match
		// the cursor position with the corresponding visible line
		if dataRowIndex >= 0 && dataRowIndex == cursor {
			cleanLine := stripANSI(line)
			result = append(result, SelectedStyle.Width(layout.InnerWidth).Render(cleanLine))
			continue
		}

		// Non-selected data rows - apply normal text color with full width
		result = append(result, NormalStyle.Width(layout.InnerWidth).Render(line))
	}

	return strings.Join(result, "\n")
}

// =============================================================================
// Filesystem Browser Model (using bubbles/table)
// =============================================================================

// fsTableRow represents a row in the filesystem table
type fsTableRow struct {
	node   *fsNode
	isBack bool // True for ".." entry
}

// fsBrowserModel is the Bubble Tea model for filesystem browsing
type fsBrowserModel struct {
	table       table.Model
	rows        []fsTableRow
	root        *fsNode
	currentNode *fsNode
	layerInfo   string
	imageRef    string
	layerDigest string
	layerSize   int64
	statusMsg   string
	quitting    bool
	layout      Layout
}

func newFSBrowserModel(entries []api.TarEntry, layerInfo, imageRef, layerDigest string, layerSize int64) fsBrowserModel {
	root := buildFSTree(entries)

	m := fsBrowserModel{
		root:        root,
		currentNode: root,
		layerInfo:   layerInfo,
		imageRef:    imageRef,
		layerDigest: layerDigest,
		layerSize:   layerSize,
		layout:      DefaultLayout(),
	}

	// Initialize table with root directory contents
	m.initTable()

	return m
}

func (m *fsBrowserModel) initTable() {
	// Build rows data
	m.rows = []fsTableRow{}

	// Always add ".." - at root it goes back to layer selector
	m.rows = append(m.rows, fsTableRow{isBack: true})

	// Add children
	for _, child := range m.currentNode.getSortedChildren() {
		m.rows = append(m.rows, fsTableRow{node: child})
	}

	// Build table rows
	tableRows := m.buildTableRows()

	// Calculate column widths to fill InnerWidth for full-width selector highlighting
	// Use InnerWidth directly to ensure columns fill full content area
	totalW := m.layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}
	sizeW := 15
	nameW := totalW - sizeW

	columns := []table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Size", Width: sizeW},
	}

	m.table = table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(true),
		table.WithHeight(m.layout.TableHeight),
	)

	// Apply styles matching the main TUI pattern
	ApplyTableStyles(&m.table)
}

func (m *fsBrowserModel) buildTableRows() []table.Row {
	tableRows := make([]table.Row, len(m.rows))
	for i, row := range m.rows {
		if row.isBack {
			tableRows[i] = table.Row{"..", ""}
		} else {
			name := row.node.name
			var info string
			if row.node.isDir {
				name += "/"
				childCount := len(row.node.children)
				if childCount == 1 {
					info = "1 item"
				} else {
					info = fmt.Sprintf("%d items", childCount)
				}
			} else {
				info = api.HumanReadableSize(row.node.size)
			}
			tableRows[i] = table.Row{name, info}
		}
	}
	return tableRows
}

func (m *fsBrowserModel) updateTableRows() {
	// Build rows data
	m.rows = []fsTableRow{}

	// Always add ".." - at root it goes back to layer selector
	m.rows = append(m.rows, fsTableRow{isBack: true})

	// Add children
	for _, child := range m.currentNode.getSortedChildren() {
		m.rows = append(m.rows, fsTableRow{node: child})
	}

	// Update table rows
	tableRows := m.buildTableRows()
	m.table.SetRows(tableRows)
}

func (m *fsBrowserModel) rebuildTable() {
	// Recalculate column widths to fill InnerWidth for full-width selector highlighting
	// Use InnerWidth directly to ensure columns fill full content area
	totalW := m.layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}
	sizeW := 15
	nameW := totalW - sizeW

	columns := []table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Size", Width: sizeW},
	}
	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func (m fsBrowserModel) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), tea.ClearScreen)
}

// downloadMsg is sent when a download completes
type downloadMsg struct {
	path string
	err  error
}

func (m fsBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case downloadMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Download failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Downloaded to: %s", msg.path)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.rebuildTable()
		return m, nil

	case tea.KeyMsg:
		// Clear status message on any key press
		m.statusMsg = ""

		switch msg.String() {
		case "q", "esc":
			// Go back - if at root, return to layer selector; otherwise go up
			if m.currentNode.parent != nil {
				m.currentNode = m.currentNode.parent
				m.updateTableRows()
				m.table.SetCursor(0)
				return m, nil
			}
			// At root - go back to layer selector
			m.quitting = true
			return m, tea.Quit
		case "d":
			// Download the layer
			m.statusMsg = "Downloading layer..."
			return m, m.downloadLayer()
		case "enter":
			cursor := m.table.Cursor()
			if cursor < 0 || cursor >= len(m.rows) {
				return m, nil
			}

			row := m.rows[cursor]
			if row.isBack {
				// Go up to parent, or back to layer selector if at root
				if m.currentNode.parent != nil {
					m.currentNode = m.currentNode.parent
					m.updateTableRows()
					m.table.SetCursor(0)
				} else {
					// At root - go back to layer selector
					m.quitting = true
					return m, tea.Quit
				}
			} else if row.node != nil && row.node.isDir {
				// Navigate into directory
				m.currentNode = row.node
				m.updateTableRows()
				m.table.SetCursor(0)
			}
			// Files: no action (could show file info in future)
			return m, nil
		case "backspace", "h":
			// Go up to parent, or back to layer selector if at root
			if m.currentNode.parent != nil {
				m.currentNode = m.currentNode.parent
				m.updateTableRows()
				m.table.SetCursor(0)
			} else {
				// At root - go back to layer selector
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m fsBrowserModel) downloadLayer() tea.Cmd {
	return func() tea.Msg {
		client := api.NewRegistryClient()
		path, err := client.DownloadLayerBlob(m.imageRef, m.layerDigest, m.layerSize)
		return downloadMsg{path: path, err: err}
	}
}

func (m fsBrowserModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Add top margin to avoid terminal edge (matching main TUI)
	b.WriteString("\n")

	// Build content inside border
	var contentBuilder strings.Builder

	// Title with layer info
	contentBuilder.WriteString(TitleStyle.Render(m.layerInfo))
	contentBuilder.WriteString("\n")
	// White divider after title
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	// Current path
	path := m.currentNode.getPath()
	if m.currentNode == m.root {
		path = "/"
	}
	contentBuilder.WriteString(NormalStyle.Render(path))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(StatsStyle.Render(fmt.Sprintf("%d items", len(m.rows)-1))) // -1 for ".."
	contentBuilder.WriteString("\n\n")

	// Table view with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

	// Show status message if present
	if m.statusMsg != "" {
		contentBuilder.WriteString("\n" + StatusMsgStyle.Render(m.statusMsg))
	}

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Add border around content (using InnerWidth so total = ViewportWidth)
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())
	b.WriteString(borderedContent)
	b.WriteString("\n")

	// Help footer below border
	b.WriteString(" " + HintStyle.Render("enter: open | backspace: up | d: download | q/esc: back"))

	return b.String()
}

// runFSBrowser launches the filesystem browser for layer contents
func runFSBrowser(entries []api.TarEntry, layerInfo, imageRef, layerDigest string, layerSize int64) error {
	m := newFSBrowserModel(entries, layerInfo, imageRef, layerDigest, layerSize)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// =============================================================================
// Layer Selector TUI (shows build steps and layers with red border)
// =============================================================================

// layerSelectorModel is the TUI model for selecting layers to inspect
// Uses bubbles/table for proper scrolling with many layers
type layerSelectorModel struct {
	table      table.Model
	imageRef   string
	layers     []api.Layer
	buildSteps []string
	selected   string // The selection result (e.g., "ALL", "0,1,2", etc.)
	inputMode  bool   // True when typing custom selection
	inputText  string
	quitting   bool
	layout     Layout

	// Pagination state - 0 = Layers, 1 = Build Steps
	currentPage   int
	buildViewport viewport.Model // viewport for scrollable build steps
	viewportReady bool
	paginator     paginator.Model // visual page indicator (dots)
}

func newLayerSelectorModel(imageRef string, layers []api.Layer, buildSteps []string) layerSelectorModel {
	layout := DefaultLayout()

	// Calculate column widths to fill InnerWidth for full-width selector highlighting
	// Use InnerWidth directly to ensure columns fill full content area
	totalW := layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}

	// Column widths: Index (8), Digest (variable), Size (12)
	indexW := 8
	sizeW := 12
	digestW := totalW - indexW - sizeW
	if digestW < 20 {
		digestW = 20
	}

	columns := []table.Column{
		{Title: "Index", Width: indexW},
		{Title: "Digest", Width: digestW},
		{Title: "Size", Width: sizeW},
	}

	// Build table rows - "ALL" first, then individual layers
	rows := make([]table.Row, len(layers)+1)
	rows[0] = table.Row{"ALL", "(fetch all layers)", ""}

	for i, layer := range layers {
		digestShort := layer.Digest
		if len(digestShort) > digestW-2 {
			digestShort = digestShort[:digestW-5] + "..."
		}
		rows[i+1] = table.Row{
			fmt.Sprintf("[%d]", i),
			digestShort,
			api.HumanReadableSize(layer.Size),
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	// Apply standard table styles for consistent look
	ApplyTableStyles(&t)

	// Ensure cursor starts at the top (first row) for proper viewport positioning
	t.GotoTop()

	// Create viewport for build steps - use full width
	vp := viewport.New(layout.InnerWidth, layout.TableHeight)

	// Build the content for the viewport - sanitize and wrap build steps
	var buildContent strings.Builder
	wrapWidth := layout.InnerWidth - 6 // Leave room for "[xx] " prefix
	for i, step := range buildSteps {
		displayStep := sanitizeBuildStep(step)
		wrapped := wrapBuildStep(displayStep, wrapWidth)
		lines := strings.Split(wrapped, "\n")
		for j, line := range lines {
			if j == 0 {
				buildContent.WriteString(fmt.Sprintf("[%d] %s\n", i, line))
			} else {
				buildContent.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
	}
	vp.SetContent(buildContent.String())

	// Create paginator for page indicator (dots)
	// Only show if there are build steps (2 pages: Layers, Build Steps)
	p := paginator.New()
	p.Type = paginator.Dots
	p.PerPage = 1
	if len(buildSteps) > 0 {
		p.SetTotalPages(2)
	} else {
		p.SetTotalPages(1)
	}

	return layerSelectorModel{
		table:         t,
		imageRef:      imageRef,
		layers:        layers,
		buildSteps:    buildSteps,
		inputMode:     false,
		layout:        layout,
		currentPage:   0, // Start on Layers page
		buildViewport: vp,
		viewportReady: true,
		paginator:     p,
	}
}

func (m *layerSelectorModel) updateTableSize() {
	// Recalculate column widths to fill InnerWidth for full-width selector highlighting
	// Use InnerWidth directly to ensure columns fill full content area
	totalW := m.layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}

	indexW := 8
	sizeW := 12
	digestW := totalW - indexW - sizeW
	if digestW < 20 {
		digestW = 20
	}

	columns := []table.Column{
		{Title: "Index", Width: indexW},
		{Title: "Digest", Width: digestW},
		{Title: "Size", Width: sizeW},
	}
	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func (m layerSelectorModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m layerSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		// Update viewport dimensions for build steps
		m.buildViewport.Width = m.layout.InnerWidth
		m.buildViewport.Height = m.layout.TableHeight
		return m, nil

	case tea.KeyMsg:
		// Handle input mode (custom layer selection)
		if m.inputMode {
			switch msg.String() {
			case "esc":
				m.inputMode = false
				m.inputText = ""
				return m, nil
			case "enter":
				if m.inputText != "" {
					m.selected = m.inputText
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			case "backspace":
				if len(m.inputText) > 0 {
					m.inputText = m.inputText[:len(m.inputText)-1]
				}
				return m, nil
			default:
				// Only accept digits and commas
				if len(msg.String()) == 1 {
					ch := msg.String()[0]
					if (ch >= '0' && ch <= '9') || ch == ',' {
						m.inputText += msg.String()
					}
				}
				return m, nil
			}
		}

		// Handle page switching with left/right arrows
		switch msg.String() {
		case "left", "h":
			if m.currentPage > 0 {
				m.currentPage--
				m.paginator.PrevPage()
				m.buildViewport.GotoTop() // Reset viewport when switching pages
			}
			return m, nil
		case "right", "l":
			// Only allow switching to build steps page if there are build steps
			if len(m.buildSteps) > 0 && m.currentPage < 1 {
				m.currentPage++
				m.paginator.NextPage()
			}
			return m, nil
		case "tab":
			// Toggle between pages
			if len(m.buildSteps) > 0 {
				m.currentPage = (m.currentPage + 1) % 2
				m.paginator.Page = m.currentPage
				m.buildViewport.GotoTop()
			}
			return m, nil
		}

		// Handle page-specific keys
		if m.currentPage == 1 {
			// Build Steps page - delegate to viewport for scrolling
			switch msg.String() {
			case "q", "esc":
				m.quitting = true
				return m, tea.Quit
			}
			// Let viewport handle all other keys (up/down/pgup/pgdn/home/end)
			var cmd tea.Cmd
			m.buildViewport, cmd = m.buildViewport.Update(msg)
			return m, cmd
		}

		// Layers page - handle layer selection
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			cursor := m.table.Cursor()
			if cursor == 0 {
				// "ALL" option
				m.selected = "ALL"
			} else {
				// Single layer selection (cursor-1 because row 0 is "ALL")
				m.selected = fmt.Sprintf("%d", cursor-1)
			}
			m.quitting = true
			return m, tea.Quit
		case "c":
			// Custom selection mode
			m.inputMode = true
			m.inputText = ""
			return m, nil
		}
	}

	// Let the table handle navigation (up/down/pgup/pgdown/home/end) on Layers page
	if m.currentPage == 0 {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m layerSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render(fmt.Sprintf("Layer Inspector: %s", m.imageRef)))
	contentBuilder.WriteString("\n")

	// Page indicator using bubbles paginator (dots)
	if len(m.buildSteps) > 0 {
		// Show page labels with paginator dots
		pageLabel := "Layers"
		if m.currentPage == 1 {
			pageLabel = "Build Steps"
		}
		paginatorLine := fmt.Sprintf("  %s  %s  (←/→ to switch)", pageLabel, m.paginator.View())
		contentBuilder.WriteString(HintStyle.Render(paginatorLine))
	}
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	if m.currentPage == 0 {
		// === LAYERS PAGE ===
		contentBuilder.WriteString(NormalStyle.Bold(true).Render("Layers:"))
		contentBuilder.WriteString(fmt.Sprintf(" (%d available)\n\n", len(m.layers)))

		// Input mode display OR table
		if m.inputMode {
			contentBuilder.WriteString(NormalStyle.Render("Enter layer indices (comma-separated): "))
			contentBuilder.WriteString(m.inputText)
			contentBuilder.WriteString("_")
			contentBuilder.WriteString("\n")
		} else {
			// Table view with full-width selection
			contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))
		}
	} else {
		// === BUILD STEPS PAGE ===
		contentBuilder.WriteString(NormalStyle.Bold(true).Render("Build Steps:"))
		contentBuilder.WriteString(fmt.Sprintf(" (%d steps)\n\n", len(m.buildSteps)))

		// Render build steps with full-width styling (similar to table)
		// Get the viewport content lines and apply full-width styling
		vpContent := m.buildViewport.View()
		vpLines := strings.Split(vpContent, "\n")

		for _, line := range vpLines {
			// Pad each line to full width for consistent appearance
			// Use StringWidth for proper visible character count
			lineLen := StringWidth(line)
			displayLine := line
			if lineLen < m.layout.InnerWidth {
				displayLine += strings.Repeat(" ", m.layout.InnerWidth-lineLen)
			}
			contentBuilder.WriteString(NormalStyle.Render(displayLine))
			contentBuilder.WriteString("\n")
		}

		// Show scroll position indicator
		scrollPercent := m.buildViewport.ScrollPercent() * 100
		scrollInfo := fmt.Sprintf("%.0f%% (↑/↓ scroll, PgUp/PgDn page)", scrollPercent)
		contentBuilder.WriteString(HintStyle.Render(scrollInfo))
	}

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content using InnerWidth (content width = InnerWidth, total = ViewportWidth)
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")

	// Help text based on current page and mode
	var helpText string
	if m.inputMode {
		helpText = "Enter: confirm | Esc: cancel"
	} else if m.currentPage == 0 {
		if len(m.buildSteps) > 0 {
			helpText = "↑/↓: navigate | Enter: select | c: custom | ←/→: switch tab | q/Esc: back"
		} else {
			helpText = "↑/↓: navigate | Enter: select | c: custom | q/Esc: back"
		}
	} else {
		helpText = "↑/↓: scroll | PgUp/PgDn: page | ←/→: switch tab | q/Esc: back"
	}
	result.WriteString(" " + HintStyle.Render(helpText))

	return result.String()
}

// runLayerSelectorTUI runs the layer selector TUI and returns the selection
func runLayerSelectorTUI(imageRef string, layers []api.Layer, buildSteps []string) (string, error) {
	m := newLayerSelectorModel(imageRef, layers, buildSteps)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("layer selector error: %w", err)
	}

	result := finalModel.(layerSelectorModel)
	return result.selected, nil
}

// RunLayerInspector runs the interactive layer inspector for a Docker image (no DB)
func RunLayerInspector(imageRef string) error {
	return runLayerInspectorInternal(imageRef, nil)
}

// RunLayerInspectorWithDB runs the layer inspector with database logging
func RunLayerInspectorWithDB(imageRef string, database *db.DB) error {
	return runLayerInspectorInternal(imageRef, database)
}

// runLayerInspectorInternal is the internal implementation
func runLayerInspectorInternal(imageRef string, database *db.DB) error {
	client := api.NewRegistryClient()

	// 1. Fetch manifest with spinner
	var manifest *api.Manifest
	var fetchErr error

	err := spinner.New().
		Title("Fetching manifest for " + imageRef + "...").
		Action(func() {
			manifest, fetchErr = client.GetManifest(imageRef, "")
		}).
		Run()

	if err != nil {
		return fmt.Errorf("spinner error: %w", err)
	}
	if fetchErr != nil {
		return fmt.Errorf("failed to fetch manifest: %w", fetchErr)
	}

	// 2. Platform selection (if multi-arch)
	if len(manifest.Platforms) > 0 {
		platformIdx, err := runPlatformSelectorTUI(manifest.Platforms)
		if err != nil {
			return fmt.Errorf("platform selection error: %w", err)
		}
		if platformIdx < 0 {
			return nil // User cancelled
		}

		// Re-fetch platform-specific manifest
		selectedPlatform := manifest.Platforms[platformIdx]
		err = spinner.New().
			Title(fmt.Sprintf("Fetching %s/%s manifest...", selectedPlatform.OS, selectedPlatform.Architecture)).
			Action(func() {
				manifest, fetchErr = client.GetManifest(imageRef, selectedPlatform.Digest)
			}).
			Run()

		if err != nil {
			return fmt.Errorf("spinner error: %w", err)
		}
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch platform manifest: %w", fetchErr)
		}
	}

	// 3. Fetch and display build steps
	var steps []string
	var configDigest string

	if manifest.Config.Digest != "" {
		configDigest = manifest.Config.Digest
		// v2 manifest - fetch config blob for build steps
		var stepsErr error
		err := spinner.New().
			Title("Fetching build steps...").
			Action(func() {
				steps, stepsErr = client.FetchBuildSteps(imageRef, manifest.Config.Digest)
			}).
			Run()

		if err != nil {
			return fmt.Errorf("spinner error: %w", err)
		}
		if stepsErr != nil {
			// Log but don't fail - build steps are optional
			fmt.Printf("  (Could not fetch build steps: %v)\n", stepsErr)
		}
	} else if len(manifest.V1History) > 0 {
		// v1 manifest - extract build steps from v1Compatibility history
		steps = api.ExtractV1BuildSteps(manifest.V1History)
	}

	// 4. Save manifest info (including build steps) to database
	if database != nil && len(steps) > 0 {
		// Calculate total size
		var totalSize int64
		for _, layer := range manifest.Layers {
			totalSize += layer.Size
		}
		// Save to database (platform is empty if not multi-arch)
		if err := database.SaveImageManifest(imageRef, "", steps, configDigest, len(manifest.Layers), totalSize); err != nil {
			fmt.Printf("  (Could not save manifest: %v)\n", err)
		}
	}

	// 5. Display layers list (exactly like Python: lines 136-139)
	if len(manifest.Layers) == 0 {
		fmt.Println("No layers found in manifest.")
		return nil
	}

	// 6. Run the layer selector TUI with proper styling
	selectionInput, err := runLayerSelectorTUI(imageRef, manifest.Layers, steps)
	if err != nil {
		return err
	}
	if selectionInput == "" {
		return nil // User cancelled
	}

	// Parse selection (like Python lines 144-147)
	var indicesToPeek []int
	selectionInput = strings.TrimSpace(selectionInput)
	if selectionInput == "" || strings.ToUpper(selectionInput) == "ALL" {
		// Default to ALL
		for i := range manifest.Layers {
			indicesToPeek = append(indicesToPeek, i)
		}
	} else {
		// Parse comma-separated indices
		parts := strings.Split(selectionInput, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			var idx int
			if _, err := fmt.Sscanf(p, "%d", &idx); err == nil {
				if idx >= 0 && idx < len(manifest.Layers) {
					indicesToPeek = append(indicesToPeek, idx)
				}
			}
		}
	}

	if len(indicesToPeek) == 0 {
		fmt.Println("No valid layer indices selected.")
		return nil
	}

	// 6. Batch mode vs Interactive mode
	// Batch mode: fetch all layers upfront, then browse without prompts
	isBatchMode := selectionInput == "" || strings.ToUpper(selectionInput) == "ALL"

	if isBatchMode && database != nil {
		// Batch mode with database - fetch all uncached layers first

		// Check which layers are already cached
		var uncachedIndices []int
		var uncachedLayers []api.Layer
		for _, idx := range indicesToPeek {
			layer := manifest.Layers[idx]
			existing, err := database.GetLayerInspectionByDigest(layer.Digest)
			if err != nil || existing == nil || existing.Contents == "" {
				uncachedIndices = append(uncachedIndices, idx)
				uncachedLayers = append(uncachedLayers, layer)
			}
		}

		if len(uncachedLayers) > 0 {
			// Run batch fetch TUI with proper theming
			fetchErrs := runBatchFetchTUI(client, imageRef, uncachedIndices, uncachedLayers, database)

			// Report any errors briefly
			if len(fetchErrs) > 0 {
				// Errors were shown in the TUI, just continue
			}
		}

		// Launch batch browser for all layers
		return showBatchLayersForImage(database, imageRef, indicesToPeek, manifest.Layers)
	}

	// Interactive mode - existing behavior for specific selections or no database
	currentIdx := 0
	for {
		if currentIdx >= len(indicesToPeek) {
			currentIdx = 0 // wrap around
		}
		if currentIdx < 0 {
			currentIdx = len(indicesToPeek) - 1
		}

		idx := indicesToPeek[currentIdx]
		layer := manifest.Layers[idx]

		var entries []api.TarEntry
		var fromCache bool

		// Check if we've already fetched this layer's contents (by digest only - same layer can appear in multiple images)
		if database != nil {
			existing, err := database.GetLayerInspectionByDigest(layer.Digest)
			if err == nil && existing != nil && existing.Contents != "" {
				// Deserialize cached entries
				if err := json.Unmarshal([]byte(existing.Contents), &entries); err != nil {
					entries = nil
				} else {
					fromCache = true
				}
			}
		}

		// If no cached entries, fetch from registry
		if entries == nil {
			var peekErr error

			err := spinner.New().
				Title(fmt.Sprintf("Fetching layer %d from registry...", idx)).
				Action(func() {
					entries, peekErr = client.PeekLayerBlob(imageRef, layer.Digest)
				}).
				Run()

			if err != nil {
				return fmt.Errorf("spinner error: %w", err)
			}

			if peekErr != nil {
				fmt.Printf("Error fetching layer: %v\n", peekErr)
				currentIdx++
				continue
			}

			// Save to database (if available)
			if database != nil {
				contentsJSON, _ := json.Marshal(entries)
				if err := database.SaveLayerInspection(imageRef, layer.Digest, idx, layer.Size, len(entries), string(contentsJSON)); err != nil {
					fmt.Printf("(Could not save to database: %v)\n", err)
				}
			}
		}

		// Display layer contents in filesystem browser
		sourceLabel := "fetched"
		if fromCache {
			sourceLabel = "cached"
		}
		sizeStr := api.HumanReadableSize(layer.Size)
		layerInfo := fmt.Sprintf("Layer %d/%d (%s) [%s] %s", currentIdx+1, len(indicesToPeek), sourceLabel, layer.Digest[:12], sizeStr)

		if err := runFSBrowser(entries, layerInfo, imageRef, layer.Digest, layer.Size); err != nil {
			return fmt.Errorf("browser error: %w", err)
		}

		// Prompt for next action
		action, err := runLayerActionSelectorTUI(currentIdx+1, len(indicesToPeek), sourceLabel)
		if err != nil || action == "" {
			// Treat error or cancellation as "done browsing"
			return nil
		}

		switch action {
		case "next":
			currentIdx++
		case "prev":
			currentIdx--
		case "refresh":
			// Clear cache and re-fetch
			if database != nil {
				// Force re-fetch by fetching fresh
				var peekErr error
				err := spinner.New().
					Title(fmt.Sprintf("Re-fetching layer %d from registry...", idx)).
					Action(func() {
						entries, peekErr = client.PeekLayerBlob(imageRef, layer.Digest)
					}).
					Run()

				if err != nil {
					fmt.Printf("Spinner error: %v\n", err)
				} else if peekErr != nil {
					fmt.Printf("Error fetching layer: %v\n", peekErr)
				} else {
					contentsJSON, _ := json.Marshal(entries)
					database.SaveLayerInspection(imageRef, layer.Digest, idx, layer.Size, len(entries), string(contentsJSON))
				}
			}
			// Stay on same layer to show refreshed content
		case "done":
			return nil
		}
	}
}

// =============================================================================
// Image Reference Input TUI (bubbles/textinput, matching app style)
// =============================================================================

// imageRefInputModel is the TUI model for entering an image reference
type imageRefInputModel struct {
	textInput textinput.Model
	done      bool
	cancelled bool
	layout    Layout
}

func newImageRefInputModel() imageRefInputModel {
	ti := textinput.New()
	ti.Placeholder = "moby/buildkit:latest"
	ti.Focus()
	ti.CharLimit = 200
	// ti.Width is set dynamically in Update() on tea.WindowSizeMsg

	layout := DefaultLayout()
	// Set initial width based on default layout
	ti.Width = layout.InnerWidth - 10

	return imageRefInputModel{
		textInput: ti,
		layout:    layout,
	}
}

func (m imageRefInputModel) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), textinput.Blink)
}

func (m imageRefInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.textInput.Width = m.layout.InnerWidth - 10
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m imageRefInputModel) View() string {
	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Enter Image Reference"))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(DimStyle.Render("Format: user/repo:tag (e.g., moby/buildkit:latest)"))
	contentBuilder.WriteString("\n\n")

	// Text input
	contentBuilder.WriteString(m.textInput.View())

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("Enter: confirm | Esc: cancel"))

	return result.String()
}

// PromptForImageRef prompts the user for an image reference
func PromptForImageRef() (string, error) {
	model := newImageRefInputModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("input error: %w", err)
	}

	result := finalModel.(imageRefInputModel)
	if result.cancelled {
		return "", nil
	}

	imageRef := strings.TrimSpace(result.textInput.Value())
	// Default to moby/buildkit:latest if empty
	if imageRef == "" {
		imageRef = "moby/buildkit:latest"
	}

	return imageRef, nil
}

// =============================================================================
// Platform Selector TUI (bubbles/table, matching app style)
// =============================================================================

// platformSelectorModel is the TUI model for selecting a platform using a table
type platformSelectorModel struct {
	table     table.Model
	platforms []api.Platform
	selected  int
	quitting  bool
	layout    Layout
}

func newPlatformSelectorModel(platforms []api.Platform) platformSelectorModel {
	layout := DefaultLayout()

	// Build table rows
	rows := make([]table.Row, len(platforms))
	for i, p := range platforms {
		label := fmt.Sprintf("%s/%s", p.OS, p.Architecture)
		if p.Variant != "" {
			label += "/" + p.Variant
		}
		rows[i] = table.Row{label}
	}

	// Single column for platform names - use TableWidth from Layout
	colWidth := layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Platform", Width: colWidth},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	// Apply styles matching the app pattern
	ApplyTableStyles(&t)

	return platformSelectorModel{
		table:     t,
		platforms: platforms,
		selected:  -1,
		layout:    layout,
	}
}

func (m platformSelectorModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m platformSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.selected = m.table.Cursor()
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *platformSelectorModel) updateTableSize() {
	// Use TableWidth from Layout
	colWidth := m.layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Platform", Width: colWidth},
	}
	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func (m platformSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Select Platform"))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("%d platforms available", len(m.platforms))))
	contentBuilder.WriteString("\n\n")

	// Table with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("↑/↓: navigate | Enter: select | q/Esc: back"))

	return result.String()
}

// runPlatformSelectorTUI runs the platform selector TUI and returns the selected index
func runPlatformSelectorTUI(platforms []api.Platform) (int, error) {
	model := newPlatformSelectorModel(platforms)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return -1, fmt.Errorf("platform selector error: %w", err)
	}

	result := finalModel.(platformSelectorModel)
	return result.selected, nil
}

// =============================================================================
// Layer Action Selector TUI (bubbles/table, matching app style)
// =============================================================================

// layerActionSelectorModel is the TUI model for selecting next action
type layerActionSelectorModel struct {
	table        table.Model
	actions      []string
	actionLabels []string
	title        string
	selected     string
	quitting     bool
	layout       Layout
}

func newLayerActionSelectorModel(currentLayer, totalLayers int, sourceLabel string) layerActionSelectorModel {
	layout := DefaultLayout()

	// Define actions and their labels
	actions := []string{"next", "prev", "refresh", "done"}
	actionLabels := []string{
		"Next layer",
		"Previous layer",
		"Re-fetch from registry",
		"Done browsing",
	}

	// Build table rows
	rows := make([]table.Row, len(actionLabels))
	for i, label := range actionLabels {
		rows[i] = table.Row{label}
	}

	// Single column for action names - use TableWidth from Layout
	colWidth := layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Action", Width: colWidth},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(rows)+2),
	)

	// Apply styles matching the app pattern
	ApplyTableStyles(&t)

	return layerActionSelectorModel{
		table:        t,
		actions:      actions,
		actionLabels: actionLabels,
		title:        fmt.Sprintf("Layer %d/%d (%s) - What next?", currentLayer, totalLayers, sourceLabel),
		layout:       layout,
	}
}

func (m layerActionSelectorModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m layerActionSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.selected = "done" // Treat escape as done
			m.quitting = true
			return m, tea.Quit
		case "enter":
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.actions) {
				m.selected = m.actions[cursor]
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *layerActionSelectorModel) updateTableSize() {
	// Use TableWidth from Layout
	colWidth := m.layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Action", Width: colWidth},
	}
	m.table.SetColumns(columns)
}

func (m layerActionSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render(m.title))
	contentBuilder.WriteString("\n\n")

	// Table with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("↑/↓: navigate | Enter: select | q/Esc: done"))

	return result.String()
}

// runLayerActionSelectorTUI runs the action selector TUI and returns the selected action
func runLayerActionSelectorTUI(currentLayer, totalLayers int, sourceLabel string) (string, error) {
	model := newLayerActionSelectorModel(currentLayer, totalLayers, sourceLabel)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("action selector error: %w", err)
	}

	result := finalModel.(layerActionSelectorModel)
	return result.selected, nil
}

// =============================================================================
// Tag Selector TUI (bubbles/table, matching app style)
// =============================================================================

// tagSelectorModel is the TUI model for selecting a tag using a table
type tagSelectorModel struct {
	table     table.Model
	tags      []string
	imageName string
	selected  string
	quitting  bool
	layout    Layout
}

func newTagSelectorModel(imageName string, tags []string) tagSelectorModel {
	layout := DefaultLayout()

	// Build table rows
	rows := make([]table.Row, len(tags))
	for i, tag := range tags {
		rows[i] = table.Row{tag}
	}

	// Single column for tag names - use TableWidth from Layout
	colWidth := layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Tag", Width: colWidth},
	}

	// Calculate table height for tag selector's specific layout:
	// Content before table: title(1) + newline(1) + divider(1) + newline(1) + tagCount(1) + doubleNewline(2) = 7
	// Box overhead: main box borders(2) + footer box(3) + spacing(1) = 6
	// Table render margin (bubbles/table header chrome): 4
	const (
		contentBeforeTable   = 7
		boxOverhead          = 6
		tagTableRenderMargin = 4
		minTagTableHeight    = 5
	)
	tableHeight := layout.ViewportHeight - contentBeforeTable - boxOverhead + tagTableRenderMargin
	if tableHeight < minTagTableHeight {
		tableHeight = minTagTableHeight
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(tableHeight),
	)

	// Apply styles matching the app pattern
	ApplyTableStyles(&t)

	return tagSelectorModel{
		table:     t,
		tags:      tags,
		imageName: imageName,
		layout:    layout,
	}
}

func (m tagSelectorModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m tagSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.tags) {
				m.selected = m.tags[cursor]
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *tagSelectorModel) updateTableSize() {
	// Use TableWidth from Layout
	colWidth := m.layout.TableWidth
	if colWidth < 20 {
		colWidth = 20
	}

	columns := []table.Column{
		{Title: "Tag", Width: colWidth},
	}
	m.table.SetColumns(columns)

	const (
		contentBeforeTable   = 7
		boxOverhead          = 6
		tagTableRenderMargin = 4
		minTagTableHeight    = 5
	)
	tableHeight := m.layout.ViewportHeight - contentBeforeTable - boxOverhead + tagTableRenderMargin
	if tableHeight < minTagTableHeight {
		tableHeight = minTagTableHeight
	}
	m.table.SetHeight(tableHeight)
}

func (m tagSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render(fmt.Sprintf("Select tag for %s", m.imageName)))
	contentBuilder.WriteString("\n")
	// White divider after title
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("%d tags available", len(m.tags))))
	contentBuilder.WriteString("\n\n")

	// Table with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

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
	helpText := "↑/↓: navigate | Enter: select | q/Esc: back"
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

// =============================================================================
// Tag Input TUI (bubbles/textinput, for fallback when tags can't be fetched)
// =============================================================================

// tagInputModel is the TUI model for manually entering a tag
type tagInputModel struct {
	textInput textinput.Model
	imageName string
	errorMsg  string
	done      bool
	cancelled bool
	layout    Layout
}

func newTagInputModel(imageName string, fetchErr error) tagInputModel {
	ti := textinput.New()
	ti.Placeholder = "latest"
	ti.Focus()
	ti.CharLimit = 100
	// ti.Width is set dynamically in Update() on tea.WindowSizeMsg

	errMsg := ""
	if fetchErr != nil {
		errMsg = fmt.Sprintf("Could not fetch tags: %v", fetchErr)
	}

	layout := DefaultLayout()
	// Set initial width based on default layout
	ti.Width = layout.InnerWidth - 10

	return tagInputModel{
		textInput: ti,
		imageName: imageName,
		errorMsg:  errMsg,
		layout:    layout,
	}
}

func (m tagInputModel) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), textinput.Blink)
}

func (m tagInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.textInput.Width = m.layout.InnerWidth - 10
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m tagInputModel) View() string {
	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render(fmt.Sprintf("Enter tag for %s", m.imageName)))
	contentBuilder.WriteString("\n")
	if m.errorMsg != "" {
		contentBuilder.WriteString(DimStyle.Render(m.errorMsg))
		contentBuilder.WriteString("\n")
	}
	contentBuilder.WriteString("\n")

	// Text input
	contentBuilder.WriteString(m.textInput.View())

	// Calculate available height for border
	// Account for: 1 top margin + 2 border lines + 1 line after border + 1 hint line = 5 lines overhead
	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("Enter: confirm | Esc: cancel"))

	return result.String()
}

// runTagInputTUI runs the tag input TUI and returns the entered tag
func runTagInputTUI(imageName string, fetchErr error) (string, error) {
	model := newTagInputModel(imageName, fetchErr)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("input error: %w", err)
	}

	result := finalModel.(tagInputModel)
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}

	return result.textInput.Value(), nil
}

// PromptForTag prompts the user to select a tag from available tags
func PromptForTag(imageName string) (string, error) {
	client := api.NewRegistryClient()

	// Fetch available tags with a spinner
	var tags []string
	var fetchErr error

	err := spinner.New().
		Title(fmt.Sprintf("Fetching tags for %s...", imageName)).
		Action(func() {
			tags, fetchErr = client.ListTags(imageName)
		}).
		Run()

	if err != nil {
		return "", fmt.Errorf("spinner error: %w", err)
	}
	if fetchErr != nil {
		// If we can't fetch tags, fall back to manual input
		tag, err := runTagInputTUI(imageName, fetchErr)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(tag) == "" {
			tag = "latest"
		}
		return tag, nil
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("no tags found for %s", imageName)
	}

	// If only one tag, use it directly
	if len(tags) == 1 {
		return tags[0], nil
	}

	// Build options for tag selection (limit to 50 most recent)
	maxTags := 50
	if len(tags) > maxTags {
		tags = tags[len(tags)-maxTags:] // Take last N (usually most recent)
	}

	// Reverse so newest are first
	for i, j := 0, len(tags)-1; i < j; i, j = i+1, j-1 {
		tags[i], tags[j] = tags[j], tags[i]
	}

	// Run the tag selector TUI
	model := newTagSelectorModel(imageName, tags)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("tag selector error: %w", err)
	}

	result := finalModel.(tagSelectorModel)
	if result.selected == "" {
		return "", fmt.Errorf("no tag selected")
	}

	return result.selected, nil
}

// cachedImageTableModel is the Bubble Tea model for cached image selection
type cachedImageTableModel struct {
	table    table.Model
	rows     []db.CachedImageRow
	selected int
	quitting bool
	layout   Layout
}

func newCachedImageTableModel(rows []db.CachedImageRow) cachedImageTableModel {
	layout := DefaultLayout()

	// Use InnerWidth for full-width selector highlighting
	totalW := layout.InnerWidth
	if totalW < 40 {
		totalW = 40
	}
	layersW := 10
	imageW := totalW - layersW // Image column fills remaining space

	columns := []table.Column{
		{Title: "Image", Width: imageW},
		{Title: "Layers", Width: layersW},
	}

	tableRows := make([]table.Row, len(rows))
	for i, row := range rows {
		tableRows[i] = table.Row{
			row.ImageRef,
			fmt.Sprintf("%d", row.LayerCount),
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	ApplyTableStyles(&t)

	return cachedImageTableModel{
		table:    t,
		rows:     rows,
		selected: -1,
		layout:   layout,
	}
}

func (m cachedImageTableModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m cachedImageTableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.selected = m.table.Cursor()
			m.quitting = true
			return m, tea.Quit
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *cachedImageTableModel) updateTableSize() {
	// Use InnerWidth for full-width selector highlighting
	totalW := m.layout.InnerWidth
	if totalW < 40 {
		totalW = 40
	}

	layersW := 10
	imageW := totalW - layersW // Image column fills remaining space

	columns := []table.Column{
		{Title: "Image", Width: imageW},
		{Title: "Layers", Width: layersW},
	}
	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func (m cachedImageTableModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Cached Layer Inspections"))
	contentBuilder.WriteString("\n")
	// White divider after title
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("%d images in cache", len(m.rows))))
	contentBuilder.WriteString("\n\n")

	// Table with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

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
	helpText := "enter: select | q/esc: back"
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

// RunCachedLayersBrowser shows cached layer inspections from the database
func RunCachedLayersBrowser(database *db.DB) error {
	if database == nil {
		return fmt.Errorf("database not available")
	}

	for {
		// Get distinct images from cache
		rows, err := database.QueryDistinctCachedImages()
		if err != nil {
			return fmt.Errorf("failed to query cached images: %w", err)
		}

		if len(rows) == 0 {
			fmt.Println("No cached layer inspections found.")
			fmt.Println("Use Docker Hub Search to inspect image layers first.")
			return nil
		}

		// Run table selector
		model := newCachedImageTableModel(rows)
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("image table error: %w", err)
		}

		m := finalModel.(cachedImageTableModel)
		if m.selected < 0 || m.selected >= len(rows) {
			return nil // Back or invalid selection
		}

		selectedImage := rows[m.selected].ImageRef

		// Show layers for selected image
		if err := showCachedLayersForImage(database, selectedImage); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

// layerTableModel is the Bubble Tea model for layer selection using a table
type layerTableModel struct {
	table    table.Model
	layers   []db.LayerInspection
	imageRef string
	selected int
	quitting bool
	layout   Layout
}

func (m *layerTableModel) updateTableSize() {
	// Use InnerWidth for full-width selector highlighting
	totalW := m.layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}

	// Fixed widths for Layer, Size, Entries; Digest fills remaining space
	layerW := 8
	sizeW := 12
	entriesW := 10
	digestW := totalW - layerW - sizeW - entriesW

	columns := []table.Column{
		{Title: "Layer", Width: layerW},
		{Title: "Digest", Width: digestW},
		{Title: "Size", Width: sizeW},
		{Title: "Entries", Width: entriesW},
	}

	m.table.SetColumns(columns)
	m.table.SetHeight(m.layout.TableHeight)
}

func newLayerTableModel(imageRef string, layers []db.LayerInspection) layerTableModel {
	layout := DefaultLayout()

	// Use InnerWidth for full-width selector highlighting
	totalW := layout.InnerWidth
	if totalW < 50 {
		totalW = 50
	}

	layerW := 8
	sizeW := 12
	entriesW := 10
	digestW := totalW - layerW - sizeW - entriesW

	columns := []table.Column{
		{Title: "Layer", Width: layerW},
		{Title: "Digest", Width: digestW},
		{Title: "Size", Width: sizeW},
		{Title: "Entries", Width: entriesW},
	}

	// Build rows - highest layers first
	rows := make([]table.Row, len(layers))
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]
		rows[len(layers)-1-i] = table.Row{
			fmt.Sprintf("%d", layer.LayerIndex),
			layer.LayerDigest[:12],
			api.HumanReadableSize(layer.LayerSize),
			fmt.Sprintf("%d", layer.EntryCount),
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(layout.TableHeight),
	)

	// Apply standard table styles (same as cachedImageTableModel)
	ApplyTableStyles(&t)

	return layerTableModel{
		table:    t,
		layers:   layers,
		imageRef: imageRef,
		selected: -1,
		layout:   layout,
	}
}

func (m layerTableModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m layerTableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			// Convert table row index back to layer index
			cursor := m.table.Cursor()
			m.selected = len(m.layers) - 1 - cursor
			m.quitting = true
			return m, tea.Quit
		case "d":
			// Download selected layer
			cursor := m.table.Cursor()
			layerIdx := len(m.layers) - 1 - cursor
			if layerIdx >= 0 && layerIdx < len(m.layers) {
				layer := m.layers[layerIdx]
				go func() {
					api.NewRegistryClient().DownloadLayerBlob(m.imageRef, layer.LayerDigest, layer.LayerSize)
				}()
			}
			return m, nil
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m layerTableModel) View() string {
	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render(fmt.Sprintf("%s - Select Layer (%d cached)", m.imageRef, len(m.layers))))
	contentBuilder.WriteString("\n")
	// White divider after title (Bug #8 fix)
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	// Table with full-width selection
	contentBuilder.WriteString(renderTableWithFullWidthSelection(m.table, m.layout))

	// Calculate available height for border (Bug #8 fix - was missing height)
	availableHeight := m.layout.ViewportHeight - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content with BOTH width AND height (Bug #8 fix)
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("enter: browse | d: download | q/esc: back"))

	return result.String()
}

func showCachedLayersForImage(database *db.DB, imageRef string) error {
	layers, err := database.GetLayerInspectionsByImage(imageRef)
	if err != nil {
		return fmt.Errorf("failed to get layers: %w", err)
	}

	if len(layers) == 0 {
		fmt.Println("No layers found for this image.")
		return nil
	}

	for {
		model := newLayerTableModel(imageRef, layers)
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("layer table error: %w", err)
		}

		m := finalModel.(layerTableModel)
		if m.selected < 0 || m.selected >= len(layers) {
			return nil // Back or invalid selection
		}

		layer := layers[m.selected]

		// Parse contents
		var entries []api.TarEntry
		if layer.Contents != "" {
			if err := json.Unmarshal([]byte(layer.Contents), &entries); err != nil {
				return fmt.Errorf("could not parse layer contents: %w", err)
			}
		}

		// Launch filesystem browser
		sizeStr := api.HumanReadableSize(layer.LayerSize)
		layerInfo := fmt.Sprintf("Layer %d [%s] %s", layer.LayerIndex, layer.LayerDigest[:12], sizeStr)
		if err := runFSBrowser(entries, layerInfo, imageRef, layer.LayerDigest, layer.LayerSize); err != nil {
			return err
		}
		// After browsing, loop back to layer selection
	}
}

// ============================================================================
// Search Cached Layers
// ============================================================================

// searchResultItem implements list.Item for search results
type searchResultItem struct {
	result db.LayerSearchResult
}

func (i searchResultItem) Title() string {
	return i.result.FilePath
}

func (i searchResultItem) Description() string {
	shortDigest := i.result.LayerDigest
	if len(shortDigest) > 12 {
		shortDigest = shortDigest[:12]
	}
	return fmt.Sprintf("%s · Layer %d [%s]", i.result.ImageRef, i.result.LayerIndex, shortDigest)
}

func (i searchResultItem) FilterValue() string {
	return i.result.FilePath
}

// searchModel is the Bubble Tea model for searching cached layers
type searchModel struct {
	textInput string
	results   []db.LayerSearchResult
	list      list.Model
	database  *db.DB
	searching bool
	quitting  bool
	layout    Layout
	inputMode bool
	statusMsg string
}

func newSearchModel(database *db.DB) searchModel {
	layout := DefaultLayout()

	// Create empty list with default delegate styles
	// Note: Charm bubbles components require lipgloss.Style, so we use their defaults
	delegate := list.NewDefaultDelegate()

	l := list.New([]list.Item{}, delegate, layout.InnerWidth-4, layout.TableHeight)
	l.Title = "Search Cached Layers"
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false) // We do our own search
	// Use default list title style - Charm component requires lipgloss.Style

	return searchModel{
		database:  database,
		list:      l,
		inputMode: true, // Start in input mode
		layout:    layout,
	}
}

func (m searchModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.list.SetSize(m.layout.InnerWidth-4, m.layout.TableHeight)
		return m, nil

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "esc", "q":
				if m.textInput == "" {
					m.quitting = true
					return m, tea.Quit
				}
				// Clear input and show results (or exit if no results)
				m.inputMode = false
				return m, nil
			case "enter":
				if m.textInput != "" {
					// Perform search
					m.searching = true
					results, err := m.database.SearchLayerContents(m.textInput)
					m.searching = false
					if err != nil {
						m.statusMsg = fmt.Sprintf("Search error: %v", err)
					} else {
						m.results = results
						m.updateListItems()
						m.statusMsg = fmt.Sprintf("Found %d results for '%s'", len(results), m.textInput)
					}
					m.inputMode = false
				}
				return m, nil
			case "backspace":
				if len(m.textInput) > 0 {
					m.textInput = m.textInput[:len(m.textInput)-1]
				}
				return m, nil
			default:
				// Add character to input (only printable chars)
				if len(msg.String()) == 1 {
					m.textInput += msg.String()
				}
				return m, nil
			}
		}

		// Results mode
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "/":
			// Enter search mode again
			m.inputMode = true
			m.textInput = ""
			return m, nil
		}
	}

	// Update list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *searchModel) updateListItems() {
	items := make([]list.Item, len(m.results))
	for i, result := range m.results {
		items[i] = searchResultItem{result: result}
	}
	m.list.SetItems(items)
}

func (m searchModel) View() string {
	if m.quitting {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Search Cached Layers"))
	contentBuilder.WriteString("\n")
	// White divider after title
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	// Search input
	if m.inputMode {
		contentBuilder.WriteString("Search: ")
		contentBuilder.WriteString(m.textInput)
		contentBuilder.WriteString("_") // Cursor
	} else {
		contentBuilder.WriteString(fmt.Sprintf("Search: %s", m.textInput))
		contentBuilder.WriteString("\n")
		if m.statusMsg != "" {
			contentBuilder.WriteString(NormalStyle.Render(m.statusMsg))
		}
		contentBuilder.WriteString("\n\n")

		// Results list
		if len(m.results) > 0 {
			contentBuilder.WriteString(m.list.View())
		} else {
			contentBuilder.WriteString(NormalStyle.Render("No results. Press / to search again."))
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
		helpText = "Enter: search | Esc: back"
	} else {
		helpText = "/: new search | q/Esc: back"
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

// RunSearchCachedLayers runs the search interface for cached layer contents
func RunSearchCachedLayers(database *db.DB) error {
	m := newSearchModel(database)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// =============================================================================
// Batch Fetch TUI Model - Styled batch layer download with spinner
// =============================================================================

type batchFetchModel struct {
	imageRef       string
	totalLayers    int
	currentLayer   int
	currentDigest  string
	errors         []error
	done           bool
	quitting       bool
	layout         Layout
	client         *api.RegistryClient
	indices        []int
	layers         []api.Layer
	database       *db.DB
	statusMessages []string
	spinner        bubbleSpinner.Model
}

type batchFetchStartMsg struct{}
type batchFetchProgressMsg struct {
	layer  int
	digest string
}
type batchFetchCompleteMsg struct {
	errors []error
}

func newBatchFetchModel(client *api.RegistryClient, imageRef string, indices []int, layers []api.Layer, database *db.DB) batchFetchModel {
	// Create spinner with dots style
	// Note: Charm bubbles spinner.Style requires lipgloss.Style, so we use default
	s := bubbleSpinner.New()
	s.Spinner = bubbleSpinner.Dot
	// Use default spinner style - Charm component requires lipgloss.Style

	return batchFetchModel{
		imageRef:       imageRef,
		totalLayers:    len(layers),
		currentLayer:   0,
		client:         client,
		indices:        indices,
		layers:         layers,
		database:       database,
		layout:         DefaultLayout(),
		statusMessages: []string{},
		spinner:        s,
	}
}

func (m batchFetchModel) Init() tea.Cmd {
	return tea.Batch(
		tea.WindowSize(),
		m.spinner.Tick,
		func() tea.Msg {
			return batchFetchStartMsg{}
		},
	)
}

func (m batchFetchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		return m, nil

	case batchFetchStartMsg:
		// Start fetching in background
		return m, m.startBatchFetch()

	case batchFetchProgressMsg:
		m.currentLayer = msg.layer
		m.currentDigest = msg.digest
		return m, nil

	case batchFetchCompleteMsg:
		m.errors = msg.errors
		m.done = true
		m.quitting = true
		return m, tea.Quit

	case bubbleSpinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m batchFetchModel) startBatchFetch() tea.Cmd {
	return func() tea.Msg {
		var errors []error

		for i, layer := range m.layers {
			idx := m.indices[i]

			// Fetch layer
			entries, err := m.client.PeekLayerBlob(m.imageRef, layer.Digest)
			if err != nil {
				errors = append(errors, fmt.Errorf("layer %d (%s): %w", idx, layer.Digest[:12], err))
				continue
			}

			// Save to database
			contentsJSON, _ := json.Marshal(entries)
			if err := m.database.SaveLayerInspection(m.imageRef, layer.Digest, idx, layer.Size, len(entries), string(contentsJSON)); err != nil {
				errors = append(errors, fmt.Errorf("layer %d (save): %w", idx, err))
			}
		}

		return batchFetchCompleteMsg{errors: errors}
	}
}

func (m batchFetchModel) View() string {
	if m.quitting && m.done {
		return ""
	}

	var contentBuilder strings.Builder

	// Title
	contentBuilder.WriteString(TitleStyle.Render("Batch Layer Fetch"))
	contentBuilder.WriteString("\n")
	// White divider after title
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	// Image reference
	contentBuilder.WriteString(NormalStyle.Render("Image: "))
	contentBuilder.WriteString(AccentStyle.Render(m.imageRef))
	contentBuilder.WriteString("\n\n")

	// Progress info
	if m.done {
		successCount := m.totalLayers - len(m.errors)
		contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("Completed: %d/%d layers cached", successCount, m.totalLayers)))
		contentBuilder.WriteString("\n")
		if len(m.errors) > 0 {
			contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("Failed: %d layers", len(m.errors))))
			contentBuilder.WriteString("\n")
		}
	} else {
		// Spinner with message
		contentBuilder.WriteString(m.spinner.View())
		contentBuilder.WriteString(" ")
		contentBuilder.WriteString(NormalStyle.Render(fmt.Sprintf("Fetching %d layers from registry...", m.totalLayers)))
		contentBuilder.WriteString("\n\n")

		// Show current progress count
		contentBuilder.WriteString(DimStyle.Render(fmt.Sprintf("Layer %d of %d", m.currentLayer+1, m.totalLayers)))
		contentBuilder.WriteString("\n")
	}

	// Calculate available height for border (Bug #9 fix - was missing height)
	availableHeight := m.layout.ViewportHeight - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Border around content with BOTH width AND height (Bug #9 fix)
	borderedContent := BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())

	var result strings.Builder
	result.WriteString("\n") // Top margin
	result.WriteString(borderedContent)
	result.WriteString("\n")

	if m.done {
		result.WriteString(" " + HintStyle.Render("Press any key to continue..."))
	} else {
		result.WriteString(" " + HintStyle.Render("Fetching layers... (q to cancel)"))
	}

	return result.String()
}

// runBatchFetchTUI runs the batch fetch with a styled TUI and returns any errors
func runBatchFetchTUI(client *api.RegistryClient, imageRef string, indices []int, layers []api.Layer, database *db.DB) []error {
	m := newBatchFetchModel(client, imageRef, indices, layers, database)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return []error{fmt.Errorf("TUI error: %w", err)}
	}

	result := finalModel.(batchFetchModel)
	return result.errors
}

// showBatchLayersForImage shows a layer table for batch-fetched cached layers
// This allows browsing all layers without prompts between each one
func showBatchLayersForImage(database *db.DB, imageRef string, indices []int, manifestLayers []api.Layer) error {
	// Build layer inspection list from cached data
	var layers []db.LayerInspection
	for _, idx := range indices {
		if idx < 0 || idx >= len(manifestLayers) {
			continue
		}
		layer := manifestLayers[idx]

		// Get cached inspection
		cached, err := database.GetLayerInspectionByDigest(layer.Digest)
		if err != nil || cached == nil {
			// Create a placeholder for uncached layers
			layers = append(layers, db.LayerInspection{
				ImageRef:    imageRef,
				LayerDigest: layer.Digest,
				LayerIndex:  idx,
				LayerSize:   layer.Size,
				EntryCount:  0,
				Contents:    "",
			})
		} else {
			// Use cached data but ensure correct index for this image
			layers = append(layers, db.LayerInspection{
				ID:          cached.ID,
				ImageRef:    imageRef,
				LayerDigest: cached.LayerDigest,
				LayerIndex:  idx,
				LayerSize:   cached.LayerSize,
				EntryCount:  cached.EntryCount,
				Contents:    cached.Contents,
			})
		}
	}

	if len(layers) == 0 {
		fmt.Println("No layers available to browse.")
		return nil
	}

	// Use the existing layer table browser in a loop
	for {
		model := newLayerTableModel(imageRef, layers)
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("layer table error: %w", err)
		}

		m := finalModel.(layerTableModel)
		if m.selected < 0 || m.selected >= len(layers) {
			return nil // Back or invalid selection
		}

		layer := layers[m.selected]

		// Parse contents
		var entries []api.TarEntry
		if layer.Contents != "" {
			if err := json.Unmarshal([]byte(layer.Contents), &entries); err != nil {
				fmt.Printf("Could not parse layer contents: %v\n", err)
				continue
			}
		} else {
			fmt.Printf("Layer %d has no cached contents.\n", layer.LayerIndex)
			continue
		}

		// Launch filesystem browser
		sizeStr := api.HumanReadableSize(layer.LayerSize)
		layerInfo := fmt.Sprintf("Layer %d [%s] %s", layer.LayerIndex, layer.LayerDigest[:12], sizeStr)
		if err := runFSBrowser(entries, layerInfo, imageRef, layer.LayerDigest, layer.LayerSize); err != nil {
			return err
		}
		// After browsing, loop back to layer selection (no prompt)
	}
}
