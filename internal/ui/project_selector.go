package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charming-commits/internal/db"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectResult represents the user's selection from the project selector
type ProjectResult struct {
	Action      string // "open", "create", "exit"
	ProjectPath string // path to the selected/created project database
}

// ProjectSelectorModel handles project selection UI
type ProjectSelectorModel struct {
	projects       []string // list of .db files
	cursor         int
	createMode     bool   // true when creating new project
	createInput    string // input for new project name
	result         *ProjectResult
	quitting       bool
	width          int
	height         int
}

// NewProjectSelectorModel creates a new project selector
func NewProjectSelectorModel(projects []string) ProjectSelectorModel {
	return ProjectSelectorModel{
		projects: projects,
		cursor:   0,
	}
}

func (m ProjectSelectorModel) Init() tea.Cmd {
	return nil
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
	case "q", "ctrl+c":
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

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		MarginBottom(1)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("88")).
		Bold(true).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Padding(0, 1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Italic(true).
		MarginTop(1)

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)

	b.WriteString(titleStyle.Render("Select Project"))
	b.WriteString("\n\n")

	if m.createMode {
		b.WriteString("Enter project name:\n\n")
		b.WriteString(inputStyle.Render(m.createInput + "_"))
		b.WriteString("\n\n")
		b.WriteString(hintStyle.Render("Press Enter to create, Esc to cancel"))
	} else {
		// List existing projects
		if len(m.projects) == 0 {
			b.WriteString(hintStyle.Render("No existing projects found"))
			b.WriteString("\n\n")
		} else {
			for i, proj := range m.projects {
				// Display project name without .db extension
				displayName := strings.TrimSuffix(proj, filepath.Ext(proj))
				line := fmt.Sprintf("  %s", displayName)
				if i == m.cursor {
					b.WriteString(selectedStyle.Render(line))
				} else {
					b.WriteString(normalStyle.Render(line))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Create New option
		createLine := "  + Create New Project"
		if m.cursor == len(m.projects) {
			b.WriteString(selectedStyle.Render(createLine))
		} else {
			b.WriteString(normalStyle.Render(createLine))
		}
		b.WriteString("\n")

		// Exit option
		exitLine := "  Exit"
		if m.cursor == len(m.projects)+1 {
			b.WriteString(selectedStyle.Render(exitLine))
		} else {
			b.WriteString(normalStyle.Render(exitLine))
		}
		b.WriteString("\n\n")

		b.WriteString(hintStyle.Render("Use j/k or arrows to navigate, Enter to select, q to quit"))
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2)

	return borderStyle.Render(b.String())
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
	p := tea.NewProgram(model)
	
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

