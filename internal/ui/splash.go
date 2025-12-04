package ui

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed ansiart.txt
var ansiArtRaw string

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

	// ANSI art - center each line
	artLines := strings.Split(ansiArtRaw, "\n")

	for _, line := range artLines {
		// Strip ANSI codes to measure actual visible width
		visibleLine := stripANSI(line)
		lineLen := len(visibleLine)

		// Center the line
		padding := (layout.InnerWidth - lineLen) / 2
		if padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Banner text - plain white, no styling needed
	banner := "yolosint! by thesavant42"

	// Center the banner
	bannerLen := len(banner)
	padding := (layout.InnerWidth - bannerLen) / 2
	if padding > 0 {
		b.WriteString(strings.Repeat(" ", padding))
	}
	b.WriteString(banner)
	b.WriteString("\n")

	// Calculate available height for splash content
	availableHeight := layout.ViewportHeight - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Wrap in red bordered box with proper height
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
