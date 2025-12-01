package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
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
// Filesystem Browser Model (using bubbles/list)
// =============================================================================

// fsItem implements list.Item for filesystem entries
type fsItem struct {
	node *fsNode
}

func (i fsItem) Title() string {
	return i.node.name
}

func (i fsItem) Description() string {
	return ""
}

func (i fsItem) FilterValue() string {
	return i.node.name
}

// parentItem represents the ".." entry to go back
type parentItem struct{}

func (i parentItem) Title() string       { return ".." }
func (i parentItem) Description() string { return "" }
func (i parentItem) FilterValue() string { return ".." }

// =============================================================================
// Custom Delegate for clean single-line file listing
// =============================================================================

type fsItemDelegate struct {
	width int
}

func (d fsItemDelegate) Height() int                             { return 1 }
func (d fsItemDelegate) Spacing() int                            { return 0 }
func (d fsItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d fsItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()

	// Styles - follow style guide: white text, gray for diminished, yellow only for help
	normalStyle := lipgloss.NewStyle().Foreground(ColorText)
	dirStyle := lipgloss.NewStyle().Foreground(ColorText)
	sizeStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	cursorStyle := lipgloss.NewStyle().Foreground(ColorBorder)

	// Full-width selection style for entire line
	selectedLineStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true).
		Width(d.width)

	var name, info string
	var isDir bool

	switch it := item.(type) {
	case fsItem:
		name = it.node.name
		isDir = it.node.isDir
		if isDir {
			name += "/"
			childCount := len(it.node.children)
			if childCount == 1 {
				info = "1 item"
			} else {
				info = fmt.Sprintf("%d items", childCount)
			}
		} else {
			info = api.HumanReadableSize(it.node.size)
		}
	case parentItem:
		name = ".."
		info = ""
		isDir = true
	}

	// Calculate padding for right-aligned info
	// Account for cursor (2 chars) and some buffer
	availWidth := d.width - 4
	nameLen := len(name)
	infoLen := len(info)
	padding := availWidth - nameLen - infoLen
	if padding < 2 {
		padding = 2
	}

	// Build the line content (plain text for layout calculation)
	var cursor string
	if selected {
		cursor = "> "
	} else {
		cursor = "  "
	}

	lineContent := cursor + name + strings.Repeat(" ", padding) + info

	if selected {
		// Apply full-width selection style to entire line
		fmt.Fprint(w, selectedLineStyle.Render(lineContent))
	} else {
		// Apply individual styles for unselected items
		var nameRendered string
		if isDir {
			nameRendered = dirStyle.Render(name)
		} else {
			nameRendered = normalStyle.Render(name)
		}
		infoRendered := sizeStyle.Render(info)

		line := cursorStyle.Render(cursor) + nameRendered + strings.Repeat(" ", padding) + infoRendered
		fmt.Fprint(w, line)
	}
}

// fsBrowserModel is the Bubble Tea model for filesystem browsing
type fsBrowserModel struct {
	list        list.Model
	root        *fsNode
	currentNode *fsNode
	layerInfo   string
	imageRef    string
	layerDigest string
	layerSize   int64
	statusMsg   string
	quitting    bool
	width       int
	height      int
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
	}

	// Initialize list with root directory contents
	m.updateListItems()

	return m
}

func (m *fsBrowserModel) updateListItems() {
	var items []list.Item

	// Always add ".." - at root it goes back to layer selector
	items = append(items, parentItem{})

	// Add children
	for _, child := range m.currentNode.getSortedChildren() {
		items = append(items, fsItem{node: child})
	}

	// Create delegate with current width
	delegate := fsItemDelegate{width: m.width}

	if m.list.Items() == nil {
		m.list = list.New(items, delegate, 0, 0)
		m.list.SetShowStatusBar(true)
		m.list.SetFilteringEnabled(true)
		m.list.SetShowHelp(false)  // We show our own help line
		m.list.SetShowTitle(false) // Title shown separately in View()
		// Style the list to be compliant - white text, gray for diminished
		m.list.Styles.StatusBar = lipgloss.NewStyle().Foreground(ColorTextDim)
		m.list.Styles.StatusEmpty = lipgloss.NewStyle().Foreground(ColorTextDim)
		m.list.Styles.StatusBarActiveFilter = lipgloss.NewStyle().Foreground(ColorText)
		m.list.Styles.StatusBarFilterCount = lipgloss.NewStyle().Foreground(ColorTextDim)
		m.list.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(ColorText)
		m.list.Styles.FilterCursor = lipgloss.NewStyle().Foreground(ColorBorder)
		m.list.Styles.NoItems = lipgloss.NewStyle().Foreground(ColorTextDim)
	} else {
		m.list.SetItems(items)
		m.list.SetDelegate(delegate)
	}
}

func (m fsBrowserModel) Init() tea.Cmd {
	return nil
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
	case tea.KeyMsg:
		// Clear status message on any key press
		m.statusMsg = ""

		// Handle navigation when not filtering
		if !m.list.SettingFilter() {
			switch msg.String() {
			case "q", "esc":
				// Go back - if at root, return to layer selector; otherwise go up
				if m.currentNode.parent != nil {
					m.currentNode = m.currentNode.parent
					m.updateListItems()
					m.list.Select(0)
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
				selected := m.list.SelectedItem()
				if selected == nil {
					return m, nil
				}

				switch item := selected.(type) {
				case parentItem:
					// Go up to parent, or back to layer selector if at root
					if m.currentNode.parent != nil {
						m.currentNode = m.currentNode.parent
						m.updateListItems()
						m.list.Select(0)
					} else {
						// At root - go back to layer selector
						m.quitting = true
						return m, tea.Quit
					}
				case fsItem:
					if item.node.isDir {
						// Navigate into directory
						m.currentNode = item.node
						m.updateListItems()
						m.list.Select(0)
					}
					// Files: no action (could show file info in future)
				}
				return m, nil
			case "backspace", "h":
				// Go up to parent, or back to layer selector if at root
				if m.currentNode.parent != nil {
					m.currentNode = m.currentNode.parent
					m.updateListItems()
					m.list.Select(0)
				} else {
					// At root - go back to layer selector
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Account for: title, path, blank line, border (2), footer
		headerHeight := 5
		footerHeight := 3
		listWidth := msg.Width - 6 // border (2) + padding (4)
		m.list.SetSize(listWidth, msg.Height-headerHeight-footerHeight)
		// Update delegate with new width
		m.list.SetDelegate(fsItemDelegate{width: listWidth})
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
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
	width := m.width
	if width < 46 {
		width = 46
	}

	var b strings.Builder

	// Title with layer info
	b.WriteString(TitleStyle.Render(m.layerInfo))
	b.WriteString("\n")

	// Current path
	path := m.currentNode.getPath()
	if m.currentNode == m.root {
		path = "/"
	}
	b.WriteString(NormalStyle.Render(path))
	b.WriteString("\n\n")

	// List view
	b.WriteString(m.list.View())

	// Show status message if present
	if m.statusMsg != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorHighlight).
			Padding(0, 1)
		b.WriteString("\n" + statusStyle.Render(m.statusMsg))
	}

	// Border around content - use width-2 to match content area
	borderStyle := BorderStyle.Width(width - 2)

	var result strings.Builder
	result.WriteString(borderStyle.Render(b.String()))
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("enter: open | backspace: up | d: download | q/esc: back"))

	return result.String()
}

// runFSBrowser launches the filesystem browser for layer contents
func runFSBrowser(entries []api.TarEntry, layerInfo, imageRef, layerDigest string, layerSize int64) error {
	m := newFSBrowserModel(entries, layerInfo, imageRef, layerDigest, layerSize)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
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
		var platformIdx int
		opts := make([]huh.Option[int], len(manifest.Platforms))
		for i, p := range manifest.Platforms {
			label := fmt.Sprintf("%s/%s", p.OS, p.Architecture)
			if p.Variant != "" {
				label += "/" + p.Variant
			}
			opts[i] = huh.NewOption(label, i)
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[int]().
					Title("Select platform").
					Description(fmt.Sprintf("%d platforms available", len(manifest.Platforms))).
					Options(opts...).
					Value(&platformIdx),
			),
		).WithTheme(NewAppTheme())

		if err := form.Run(); err != nil {
			return fmt.Errorf("platform selection error: %w", err)
		}

		// Re-fetch platform-specific manifest
		selectedPlatform := manifest.Platforms[platformIdx]
		err := spinner.New().
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

	if manifest.Config.Digest != "" {
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

	if len(steps) > 0 {
		fmt.Println()
		fmt.Println("Build steps:")
		for i, step := range steps {
			fmt.Printf(" [%d] %s\n", i, step)
		}
	}

	// 4. Display layers list (exactly like Python: lines 136-139)
	if len(manifest.Layers) == 0 {
		fmt.Println("No layers found in manifest.")
		return nil
	}

	fmt.Println("\nLayers:")
	for idx, layer := range manifest.Layers {
		if layer.Size > 0 {
			fmt.Printf(" [%d] %s - %s\n", idx, layer.Digest, api.HumanReadableSize(layer.Size))
		} else {
			fmt.Printf(" [%d] %s\n", idx, layer.Digest)
		}
	}

	// 5. Layer selection via text input (like Python lines 141-147)
	// "Layers to peek (comma-separated INDEX or ALL) [default: ALL]: "
	var selectionInput string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Layers to peek (comma-separated INDEX or ALL)").
				Description(fmt.Sprintf("%d layers available", len(manifest.Layers))).
				Placeholder("ALL").
				Value(&selectionInput),
		),
	).WithTheme(NewAppTheme())

	if err := form.Run(); err != nil {
		return fmt.Errorf("layer selection error: %w", err)
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
		fmt.Println()

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
			// Batch fetch uncached layers
			fmt.Printf("Batch fetching %d uncached layers...\n", len(uncachedLayers))

			var fetchErrs []error
			err := spinner.New().
				Title(fmt.Sprintf("Fetching %d layers from registry...", len(uncachedLayers))).
				Action(func() {
					fetchErrs = batchFetchLayers(client, imageRef, uncachedIndices, uncachedLayers, database)
				}).
				Run()

			if err != nil {
				fmt.Printf("Spinner error: %v\n", err)
			}

			// Report any errors
			if len(fetchErrs) > 0 {
				fmt.Printf("Warning: %d layer(s) failed to fetch:\n", len(fetchErrs))
				for _, e := range fetchErrs {
					fmt.Printf("  - %v\n", e)
				}
			}

			successCount := len(uncachedLayers) - len(fetchErrs)
			fmt.Printf("✓ %d layers cached successfully.\n", successCount)
		} else {
			fmt.Printf("✓ All %d layers already cached.\n", len(indicesToPeek))
		}

		// Launch batch browser for all layers
		fmt.Println("Launching layer browser...")
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
		var action string
		actionForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(fmt.Sprintf("Layer %d/%d (%s) - What next?", currentIdx+1, len(indicesToPeek), sourceLabel)).
					Options(
						huh.NewOption("Next layer", "next"),
						huh.NewOption("Previous layer", "prev"),
						huh.NewOption("Re-fetch from registry", "refresh"),
						huh.NewOption("Done browsing", "done"),
					).
					Value(&action),
			),
		).WithTheme(NewAppTheme())

		if err := actionForm.Run(); err != nil {
			// Treat form cancellation (esc) as "done browsing"
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

// PromptForImageRef prompts the user for an image reference
func PromptForImageRef() (string, error) {
	var imageRef string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter image reference").
				Description("Format: user/repo:tag (e.g., moby/buildkit:latest)").
				Placeholder("moby/buildkit:latest").
				Value(&imageRef),
		),
	).WithTheme(NewAppTheme())

	if err := form.Run(); err != nil {
		return "", err
	}

	// Default to moby/buildkit:latest if empty
	if strings.TrimSpace(imageRef) == "" {
		imageRef = "moby/buildkit:latest"
	}

	return imageRef, nil
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
		var tag string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("Enter tag for %s", imageName)).
					Description(fmt.Sprintf("Could not fetch tags: %v", fetchErr)).
					Placeholder("latest").
					Value(&tag),
			),
		).WithTheme(NewAppTheme())

		if err := form.Run(); err != nil {
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

	var selectedTag string
	opts := make([]huh.Option[string], len(tags))
	for i, t := range tags {
		opts[i] = huh.NewOption(t, t)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select tag for %s", imageName)).
				Description(fmt.Sprintf("%d tags available", len(tags))).
				Options(opts...).
				Value(&selectedTag).
				Height(15),
		),
	).WithTheme(NewAppTheme())

	if err := form.Run(); err != nil {
		return "", err
	}

	return selectedTag, nil
}

// cachedImageTableModel is the Bubble Tea model for cached image selection
type cachedImageTableModel struct {
	table    table.Model
	rows     []db.CachedImageRow
	selected int
	quitting bool
	width    int
	height   int
}

func newCachedImageTableModel(rows []db.CachedImageRow) cachedImageTableModel {
	columns := []table.Column{
		{Title: "Image", Width: 50},
		{Title: "Layers", Width: 10},
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
		table.WithHeight(len(rows)+4),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorText).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorText)
	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)
	s.Cell = s.Cell.Foreground(ColorText)
	t.SetStyles(s)

	return cachedImageTableModel{
		table:    t,
		rows:     rows,
		selected: -1,
	}
}

func (m cachedImageTableModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m cachedImageTableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
	// Calculate usable width inside the border (border takes 2 chars)
	// Additional 4 chars for table internal padding/margins
	innerWidth := m.width - 6
	if innerWidth < 40 {
		innerWidth = 40
	}

	layersW := 10
	imageW := innerWidth - layersW // Image column fills remaining space

	columns := []table.Column{
		{Title: "Image", Width: imageW},
		{Title: "Layers", Width: layersW},
	}
	m.table.SetColumns(columns)

	tableHeight := m.height - 8
	if tableHeight < 5 {
		tableHeight = 5
	}
	m.table.SetHeight(tableHeight)
}

func (m cachedImageTableModel) View() string {
	if m.quitting {
		return ""
	}

	width := m.width
	if width < 46 {
		width = 46 // minimum to fit table (40 inner + 6 overhead)
	}

	var b strings.Builder

	// Title
	b.WriteString(TitleStyle.Render("Cached Layer Inspections"))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render(fmt.Sprintf("%d images in cache", len(m.rows))))
	b.WriteString("\n\n")

	// Table
	b.WriteString(m.table.View())

	// Border around content - use width-2 to match table column calculations
	borderStyle := BorderStyle.Width(width - 2)

	var result strings.Builder
	result.WriteString(borderStyle.Render(b.String()))
	result.WriteString("\n")
	result.WriteString(" " + HintStyle.Render("enter: select | q/esc: back"))

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
	width    int
	height   int
}

func (m *layerTableModel) updateTableSize() {
	if m.width == 0 {
		return
	}

	// Calculate column widths based on terminal width
	// Reserve some space for borders and padding
	availWidth := m.width - 10
	if availWidth < 40 {
		availWidth = 40
	}

	// Fixed widths for Layer, Size, Entries; Digest gets the rest
	layerW := 8
	sizeW := 10
	entriesW := 10
	digestW := availWidth - layerW - sizeW - entriesW
	if digestW < 14 {
		digestW = 14
	}

	columns := []table.Column{
		{Title: "Layer", Width: layerW},
		{Title: "Digest", Width: digestW},
		{Title: "Size", Width: sizeW},
		{Title: "Entries", Width: entriesW},
	}

	m.table.SetColumns(columns)

	// Set height based on terminal height
	tableHeight := m.height - 8 // Reserve space for title and help
	if tableHeight < 5 {
		tableHeight = 5
	}
	// Don't cap - let the table show all rows if there's space
	m.table.SetHeight(tableHeight)
}

func newLayerTableModel(imageRef string, layers []db.LayerInspection) layerTableModel {
	columns := []table.Column{
		{Title: "Layer", Width: 8},
		{Title: "Digest", Width: 14},
		{Title: "Size", Width: 10},
		{Title: "Entries", Width: 10},
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
		table.WithHeight(len(layers)+4), // +4 for header row, border line, and extra padding
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorText).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorText)
	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)
	t.SetStyles(s)

	return layerTableModel{
		table:    t,
		layers:   layers,
		imageRef: imageRef,
		selected: -1,
	}
}

func (m layerTableModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m layerTableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
	width := m.width
	if width < 40 {
		width = 80
	}

	var b strings.Builder

	// Title
	b.WriteString(TitleStyle.Render(fmt.Sprintf("%s - Select Layer (%d cached)", m.imageRef, len(m.layers))))
	b.WriteString("\n\n")

	// Table
	b.WriteString(m.table.View())

	// Border around content - use width-2 to leave room for border chars
	borderStyle := BorderStyle.Width(width - 2)

	var result strings.Builder
	result.WriteString(borderStyle.Render(b.String()))
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
	width     int
	height    int
	inputMode bool
	statusMsg string
}

func newSearchModel(database *db.DB) searchModel {
	// Create empty list
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = SelectedStyle
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	delegate.Styles.NormalTitle = NormalStyle
	delegate.Styles.NormalDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	l := list.New([]list.Item{}, delegate, 80, 20)
	l.Title = "Search Cached Layers"
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false) // We do our own search
	l.Styles.Title = TitleStyle

	return searchModel{
		database:  database,
		list:      l,
		inputMode: true, // Start in input mode
	}
}

func (m searchModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-8)
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

	// Default width if not yet set by WindowSizeMsg
	width := m.width
	if width < 40 {
		width = 80
	}

	var b strings.Builder

	// Title
	b.WriteString(TitleStyle.Render("Search Cached Layers"))
	b.WriteString("\n\n")

	// Search input
	if m.inputMode {
		b.WriteString("Search: ")
		b.WriteString(m.textInput)
		b.WriteString("_") // Cursor
	} else {
		b.WriteString(fmt.Sprintf("Search: %s", m.textInput))
		b.WriteString("\n")
		if m.statusMsg != "" {
			b.WriteString(NormalStyle.Render(m.statusMsg))
		}
		b.WriteString("\n\n")

		// Results list
		if len(m.results) > 0 {
			b.WriteString(m.list.View())
		} else {
			b.WriteString(NormalStyle.Render("No results. Press / to search again."))
		}
	}

	// Border
	borderStyle := BorderStyle.Width(width)

	var result strings.Builder
	result.WriteString(borderStyle.Render(b.String()))
	result.WriteString("\n")
	if m.inputMode {
		result.WriteString(" " + HintStyle.Render("Enter: search | Esc: back"))
	} else {
		result.WriteString(" " + HintStyle.Render("/ : new search | q/Esc: back"))
	}

	return result.String()
}

// RunSearchCachedLayers runs the search interface for cached layer contents
func RunSearchCachedLayers(database *db.DB) error {
	m := newSearchModel(database)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// batchFetchLayers fetches all specified layers and caches them to the database
// Returns errors for any failed fetches (continues fetching remaining layers on error)
func batchFetchLayers(client *api.RegistryClient, imageRef string, indices []int, layers []api.Layer, database *db.DB) []error {
	var errors []error

	for i, layer := range layers {
		idx := indices[i]
		entries, err := client.PeekLayerBlob(imageRef, layer.Digest)
		if err != nil {
			errors = append(errors, fmt.Errorf("layer %d (%s): %w", idx, layer.Digest[:12], err))
			continue
		}

		// Save to database
		contentsJSON, _ := json.Marshal(entries)
		if err := database.SaveLayerInspection(imageRef, layer.Digest, idx, layer.Size, len(entries), string(contentsJSON)); err != nil {
			errors = append(errors, fmt.Errorf("layer %d (save): %w", idx, err))
		}
	}

	return errors
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
