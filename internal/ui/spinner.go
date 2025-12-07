package ui

// spinner.go provides a blocking spinner for long-running operations.
// Uses Bubble Tea spinner (white) instead of huh/spinner.

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// Blocking Spinner - Replaces huh/spinner
// =============================================================================

// actionDoneMsg signals the action completed
type actionDoneMsg struct {
	err error
}

// blockingSpinnerModel runs a spinner while an action executes
type blockingSpinnerModel struct {
	spinner spinner.Model
	title   string
	action  func()
	done    bool
	err     error
}

// RunWithSpinner executes an action while displaying a spinner.
// This is a drop-in replacement for huh/spinner.New().Title().Action().Run()
//
// Example:
//
//	var result *MyType
//	var fetchErr error
//	err := RunWithSpinner("Fetching data...", func() {
//	    result, fetchErr = api.FetchData()
//	})
//	if err != nil { return err }
//	if fetchErr != nil { return fetchErr }
func RunWithSpinner(title string, action func()) error {
	// Use NewAppSpinner from styles.go for consistent white spinner
	s := NewAppSpinner()

	m := blockingSpinnerModel{
		spinner: s,
		title:   title,
		action:  action,
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("spinner program error: %w", err)
	}

	final := finalModel.(blockingSpinnerModel)
	return final.err
}

func (m blockingSpinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.runAction(),
	)
}

func (m blockingSpinnerModel) runAction() tea.Cmd {
	return func() tea.Msg {
		m.action()
		return actionDoneMsg{}
	}
}

func (m blockingSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case actionDoneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		// Allow ctrl+c to cancel
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m blockingSpinnerModel) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), RenderNormal(m.title))
}
