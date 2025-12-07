package ui

// inputs.go provides generic text input models.
// Use these instead of creating separate imageRefInputModel, tagInputModel, etc.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// Generic Input Model
// =============================================================================

// InputConfig defines configuration for a generic text input.
type InputConfig struct {
	Title       string             // Main title displayed at top
	Subtitle    string             // Optional subtitle/description
	Placeholder string             // Placeholder text in input field
	HelpText    string             // Help text for footer
	Default     string             // Default value
	Validator   func(string) error // Optional validation function
}

// InputModel is a generic text input with two-box layout.
// Replaces imageRefInputModel, tagInputModel, dockerPathInputModel, etc.
type InputModel struct {
	textInput textinput.Model
	config    InputConfig
	layout    Layout
	value     string
	cancelled bool
	err       error
}

// NewInputModel creates a generic text input with the given configuration.
func NewInputModel(cfg InputConfig) InputModel {
	ti := textinput.New()
	ti.Placeholder = cfg.Placeholder
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = DefaultLayout().InnerWidth - 4 // Leave some padding

	if cfg.Default != "" {
		ti.SetValue(cfg.Default)
	}

	// Default help text if not provided
	if cfg.HelpText == "" {
		cfg.HelpText = "Enter: confirm | Esc: cancel"
	}

	return InputModel{
		textInput: ti,
		config:    cfg,
		layout:    DefaultLayout(),
	}
}

func (m InputModel) Init() tea.Cmd {
	return tea.Batch(
		StandardInit(),
		textinput.Blink,
	)
}

func (m InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width, msg.Height)
		m.textInput.Width = m.layout.InnerWidth - 4
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, tea.Quit

		case "enter":
			value := strings.TrimSpace(m.textInput.Value())

			// Run validator if provided
			if m.config.Validator != nil {
				if err := m.config.Validator(value); err != nil {
					m.err = err
					return m, nil
				}
			}

			m.value = value
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m InputModel) View() string {
	var content strings.Builder

	// Header
	if m.config.Subtitle != "" {
		content.WriteString(ViewHeaderWithSubtitle(m.config.Title, m.config.Subtitle, m.layout.InnerWidth))
	} else {
		content.WriteString(ViewHeader(m.config.Title, m.layout.InnerWidth))
	}

	// Input field
	content.WriteString(m.textInput.View())
	content.WriteString("\n")

	// Error message if present
	if m.err != nil {
		content.WriteString("\n")
		content.WriteString(RenderError(m.err.Error()))
		content.WriteString("\n")
	}

	return TwoBoxView(content.String(), m.config.HelpText, m.layout)
}

// Value returns the entered value after the input completes.
func (m InputModel) Value() string {
	return m.value
}

// Cancelled returns true if the user pressed Esc.
func (m InputModel) Cancelled() bool {
	return m.cancelled
}

// =============================================================================
// Convenience Functions
// =============================================================================

// RunInput runs an input TUI and returns the entered value.
// Returns empty string and cancelled=true if user pressed Esc.
func RunInput(cfg InputConfig) (value string, cancelled bool, err error) {
	model := NewInputModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", false, fmt.Errorf("input error: %w", err)
	}
	result := finalModel.(InputModel)
	return result.Value(), result.Cancelled(), nil
}

// RunInputWithDefault runs an input TUI with a default value.
func RunInputWithDefault(title, placeholder, defaultValue string) (string, bool, error) {
	return RunInput(InputConfig{
		Title:       title,
		Placeholder: placeholder,
		Default:     defaultValue,
	})
}
