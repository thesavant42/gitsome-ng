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
	width       int
	height      int
}

// NewProjectSelectorModel creates a new project selector
func NewProjectSelectorModel(projects []string) ProjectSelectorModel {
	return ProjectSelectorModel{
		projects: projects,
		cursor:   0,
		width:    DefaultWidth,
		height:   30,
	}
}

func (m ProjectSelectorModel) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m ProjectSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

	// Calculate layout dimensions
	layout := NewLayout(m.width, m.height)

	var b strings.Builder

	b.WriteString(TitleStyle.Render("Select Project"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", layout.InnerWidth))
	b.WriteString("\n\n")

	if m.createMode {
		b.WriteString("Enter project name:\n\n")
		b.WriteString(AccentStyle.Render(m.createInput + "_"))
		b.WriteString("\n\n")
		b.WriteString(HintStyle.Render("Press Enter to create, Esc to cancel"))
	} else {
		// List existing projects
		if len(m.projects) == 0 {
			b.WriteString(HintStyle.Render("No existing projects found"))
			b.WriteString("\n\n")
		} else {
			for i, proj := range m.projects {
				// Display project name without .db extension
				displayName := strings.TrimSuffix(proj, filepath.Ext(proj))
				if i == m.cursor {
					b.WriteString(SelectedStyle.Width(layout.InnerWidth).Render("• " + displayName))
				} else {
					b.WriteString(NormalStyle.Render("• " + displayName))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Create New option
		if m.cursor == len(m.projects) {
			b.WriteString(SelectedStyle.Width(layout.InnerWidth).Render("• Create New Project"))
		} else {
			b.WriteString(NormalStyle.Render("• Create New Project"))
		}
		b.WriteString("\n")

		// Exit option
		if m.cursor == len(m.projects)+1 {
			b.WriteString(SelectedStyle.Width(layout.InnerWidth).Render("• Exit"))
		} else {
			b.WriteString(NormalStyle.Render("• Exit"))
		}
		b.WriteString("\n")
	}

	// Pad content to fill available height (account for border and help line)
	content := b.String()
	contentLines := strings.Count(content, "\n")
	availableHeight := m.height - 4 // -2 for border top/bottom, -1 for help, -1 for margin
	if contentLines < availableHeight {
		content += strings.Repeat("\n", availableHeight-contentLines)
	}

	// Apply border with full width and height
	borderedContent := BorderStyle.
		Width(layout.InnerWidth).
		Height(availableHeight).
		Render(content)

	// Build final view with help text below
	var result strings.Builder
	result.WriteString(borderedContent)

	// Add help text outside the border (only in select mode)
	if !m.createMode {
		result.WriteString("\n")
		result.WriteString(" " + HintStyle.Render("up/down: navigate | Enter: select | q: quit"))
	}

	return result.String()
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
