package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/thesavant42/gitsome-ng/internal/db"

	tea "github.com/charmbracelet/bubbletea"
)

// ProjectResult represents the user's selection from the project selector
type ProjectResult struct {
	Action      string // "open", "create", "exit"
	ProjectPath string // path to the selected/created project database
}

// ProjectSelectorModel handles project selection UI
type ProjectSelectorModel struct {
	projects    []string // list of .db files
	cursor      int
	createMode  bool   // true when creating new project
	createInput string // input for new project name
	result      *ProjectResult
	quitting    bool
	layout      Layout
}

// NewProjectSelectorModel creates a new project selector
func NewProjectSelectorModel(projects []string) ProjectSelectorModel {
	return ProjectSelectorModel{
		projects: projects,
		cursor:   0,
		layout:   DefaultLayout(),
	}
}

func (m ProjectSelectorModel) Init() tea.Cmd {
	return StandardInit()
}

func (m ProjectSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if m.createMode {
			return m.handleCreateMode(msg)
		}
		return m.handleSelectMode(msg)
	}
	return m, nil
}

func (m ProjectSelectorModel) handleSelectMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalOptions := len(m.projects) + 2 // projects + "Create New" + "Exit"

	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.result = &ProjectResult{Action: "exit"}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < totalOptions-1 {
			m.cursor++
		}

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Handle numeric hotkeys for projects (1-9)
		num := int(msg.String()[0] - '0') // Convert '1'-'9' to 1-9
		index := num - 1                  // Convert to 0-based index
		if index >= 0 && index < len(m.projects) {
			// Valid project number - select and open immediately
			m.result = &ProjectResult{
				Action:      "open",
				ProjectPath: m.projects[index],
			}
			m.quitting = true
			return m, tea.Quit
		}

	case "enter":
		if m.cursor < len(m.projects) {
			// Selected existing project
			m.result = &ProjectResult{
				Action:      "open",
				ProjectPath: m.projects[m.cursor],
			}
			m.quitting = true
			return m, tea.Quit
		} else if m.cursor == len(m.projects) {
			// Create New Project
			m.createMode = true
			m.createInput = ""
		} else {
			// Exit
			m.result = &ProjectResult{Action: "exit"}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ProjectSelectorModel) handleCreateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.createMode = false
		m.createInput = ""

	case "enter":
		if m.createInput != "" {
			// Sanitize and create project name
			name := sanitizeProjectName(m.createInput)
			if name != "" {
				m.result = &ProjectResult{
					Action:      "create",
					ProjectPath: name + ".db",
				}
				m.quitting = true
				return m, tea.Quit
			}
		}

	case "backspace":
		if len(m.createInput) > 0 {
			m.createInput = m.createInput[:len(m.createInput)-1]
		}

	default:
		// Add character to input (filter special chars)
		if len(msg.String()) == 1 {
			char := msg.String()[0]
			if isValidProjectChar(char) {
				m.createInput += msg.String()
			}
		}
	}
	return m, nil
}

func (m ProjectSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Use ViewHeader helper for consistent title + divider
	b.WriteString(ViewHeader("Select Project", m.layout.InnerWidth))

	if m.createMode {
		b.WriteString("Enter project name:\n\n")
		b.WriteString(AccentStyle.Render(m.createInput + "_"))
		b.WriteString("\n\n")
		b.WriteString(HintStyle.Render("Press Enter to create, Esc to cancel"))

		// Create mode: single box layout
		mainContent := b.String()
		availableHeight := m.layout.MainContentHeight()
		paddedContent := PadToHeight(mainContent, availableHeight)

		return BorderStyle.
			Width(m.layout.InnerWidth).
			Height(availableHeight).
			Render(paddedContent)
	}

	// Select mode: render project list
	if len(m.projects) == 0 {
		b.WriteString(HintStyle.Render("No existing projects found"))
		b.WriteString("\n\n")
	} else {
		for i, proj := range m.projects {
			displayName := strings.TrimSuffix(proj, filepath.Ext(proj))
			b.WriteString(RenderNumberedItem(i+1, displayName, i == m.cursor, m.layout.InnerWidth))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Create New option
	b.WriteString(RenderListItem("Create New Project", m.cursor == len(m.projects), m.layout.InnerWidth))
	b.WriteString("\n")

	// Exit option
	b.WriteString(RenderListItem("Exit", m.cursor == len(m.projects)+1, m.layout.InnerWidth))
	b.WriteString("\n")

	// Use TwoBoxView helper for standard two-box layout
	helpText := "1-9: open project | ↑/↓: navigate | Enter: select | q: quit"
	return TwoBoxView(b.String(), helpText, m.layout)
}

// Result returns the user's selection after the program exits
func (m ProjectSelectorModel) Result() *ProjectResult {
	return m.result
}

// sanitizeProjectName removes invalid characters from project name
func sanitizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	// Remove any path separators or extension
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, ".db")
	return name
}

// isValidProjectChar returns true if the character is valid for a project name
func isValidProjectChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == ' '
}

// RunProjectSelector displays the project selection screen and returns the user's choice
func RunProjectSelector() (*ProjectResult, error) {
	// Get list of .db files in current directory
	projects, err := db.ListProjectFiles(".")
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	model := NewProjectSelectorModel(projects)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("project selector failed: %w", err)
	}

	result := finalModel.(ProjectSelectorModel).Result()
	if result == nil {
		return &ProjectResult{Action: "exit"}, nil
	}
	return result, nil
}
