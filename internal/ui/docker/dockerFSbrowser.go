package docker

// dockerFSbrowser.go - Filesystem browser for Docker layer contents
// LIPGLOSS-FREE: Uses centralized styles from ui/styles.go

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/ui"
)

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
// Filesystem Browser Model
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
	layout      ui.Layout
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
		layout:      ui.DefaultLayout(),
	}

	m.initTable()
	return m
}

func (m *fsBrowserModel) initTable() {
	m.rows = []fsTableRow{}
	m.rows = append(m.rows, fsTableRow{isBack: true})

	for _, child := range m.currentNode.getSortedChildren() {
		m.rows = append(m.rows, fsTableRow{node: child})
	}

	tableRows := m.buildTableRows()

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

	ui.ApplyTableStyles(&m.table)
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
	m.rows = []fsTableRow{}
	m.rows = append(m.rows, fsTableRow{isBack: true})

	for _, child := range m.currentNode.getSortedChildren() {
		m.rows = append(m.rows, fsTableRow{node: child})
	}

	tableRows := m.buildTableRows()
	m.table.SetRows(tableRows)
}

func (m *fsBrowserModel) rebuildTable() {
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
		m.layout = ui.NewLayout(msg.Width, msg.Height)
		m.rebuildTable()
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""

		switch msg.String() {
		case "q", "esc":
			if m.currentNode.parent != nil {
				m.currentNode = m.currentNode.parent
				m.updateTableRows()
				m.table.SetCursor(0)
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		case "d":
			m.statusMsg = "Downloading layer..."
			return m, m.downloadLayer()
		case "enter":
			cursor := m.table.Cursor()
			if cursor < 0 || cursor >= len(m.rows) {
				return m, nil
			}

			row := m.rows[cursor]
			if row.isBack {
				if m.currentNode.parent != nil {
					m.currentNode = m.currentNode.parent
					m.updateTableRows()
					m.table.SetCursor(0)
				} else {
					m.quitting = true
					return m, tea.Quit
				}
			} else if row.node != nil && row.node.isDir {
				m.currentNode = row.node
				m.updateTableRows()
				m.table.SetCursor(0)
			}
			return m, nil
		case "backspace", "h":
			if m.currentNode.parent != nil {
				m.currentNode = m.currentNode.parent
				m.updateTableRows()
				m.table.SetCursor(0)
			} else {
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
	b.WriteString("\n")

	var contentBuilder strings.Builder

	contentBuilder.WriteString(ui.TitleStyle.Render(m.layerInfo))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(strings.Repeat("─", m.layout.InnerWidth))
	contentBuilder.WriteString("\n\n")

	path := m.currentNode.getPath()
	if m.currentNode == m.root {
		path = "/"
	}
	contentBuilder.WriteString(ui.NormalStyle.Render(path))
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(ui.StatsStyle.Render(fmt.Sprintf("%d items", len(m.rows)-1)))
	contentBuilder.WriteString("\n\n")

	contentBuilder.WriteString(ui.RenderTableWithSelection(m.table, m.layout))

	if m.statusMsg != "" {
		contentBuilder.WriteString("\n" + ui.StatusMsgStyle.Render(m.statusMsg))
	}

	availableHeight := m.layout.ViewportHeight - 5
	if availableHeight < 10 {
		availableHeight = 10
	}

	borderedContent := ui.BorderStyle.
		Width(m.layout.InnerWidth).
		Height(availableHeight).
		Render(contentBuilder.String())
	b.WriteString(borderedContent)
	b.WriteString("\n")

	b.WriteString(" " + ui.HintStyle.Render("enter: open | backspace: up | d: download | q/esc: back"))

	return b.String()
}

// RunFSBrowser launches the filesystem browser for layer contents
func RunFSBrowser(entries []api.TarEntry, layerInfo, imageRef, layerDigest string, layerSize int64) error {
	m := newFSBrowserModel(entries, layerInfo, imageRef, layerDigest, layerSize)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
