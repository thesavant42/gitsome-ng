package ui

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SplashModel is the TUI model for the splash screen
type SplashModel struct {
	width  int
	height int
	done   bool
}

type splashTimeoutMsg struct{}

func waitForTimeout() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return splashTimeoutMsg{}
	})
}

func (m SplashModel) Init() tea.Cmd {
	return waitForTimeout()
}

func (m SplashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		m.done = true
		return m, tea.Quit
	case splashTimeoutMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m SplashModel) View() string {
	if m.done {
		return ""
	}

	layout := NewLayout(m.width, m.height)

	var b strings.Builder

	// Add padding at the top so the border is visible
	b.WriteString("\n")

	// Create a red square the size of the viewport with black background
	// This is a placeholder to be fixed later
	squareWidth := layout.InnerWidth
	squareHeight := layout.ViewportHeight - 4 // Account for padding

	// Calculate center position for text
	text := "YOLOSINT! by savant42"
	textWidth := len(text)
	textLine := squareHeight / 2 // Center vertically

	// Create a black square with centered text
	for i := 0; i < squareHeight; i++ {
		if i == textLine {
			// Center the text horizontally
			padding := (squareWidth - textWidth) / 2
			if padding > 0 {
				b.WriteString(strings.Repeat(" ", padding))
			}
			b.WriteString(text)
			// Fill remaining space
			remaining := squareWidth - padding - textWidth
			if remaining > 0 {
				b.WriteString(strings.Repeat(" ", remaining))
			}
		} else {
			// Create a row of black space
			b.WriteString(strings.Repeat(" ", squareWidth))
		}
		b.WriteString("\n")
	}

	// Calculate available height for the red frame
	availableHeight := layout.ViewportHeight - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Wrap the black square in a red bordered box
	borderedContent := BorderStyle.
		Width(layout.InnerWidth).
		Height(availableHeight).
		Render(b.String())

	return borderedContent
}

// ShowSplash displays the splash screen for 3 seconds
func ShowSplash() {
	model := SplashModel{
		width:  DefaultWidth,
		height: 30,
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	p.Run()

	// Clear screen before continuing
	fmt.Print("\033[2J\033[H")
}
