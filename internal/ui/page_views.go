package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
)

// page_views.go provides a fluent API for building consistent page views.
// Use PageViewBuilder to construct views with a standardized layout.

// PageViewBuilder provides a fluent API for building page views.
// It handles all the boilerplate of titles, dividers, spacing, and two-box layout.
//
// Example usage:
//
//	return NewPageView(m.layout).
//	    Title("My Page").
//	    Divider().
//	    Spacing(2).
//	    QueryInfo("Showing 10 results").
//	    Table(m.table).
//	    Status(m.statusMsg).
//	    Help("↑/↓: navigate | Enter: select").
//	    Build()
type PageViewBuilder struct {
	layout      Layout
	content     strings.Builder
	helpText    string
	hadContent  bool
	tableAdded  bool
	statusAdded bool
}

// NewPageView creates a new PageViewBuilder with the given layout.
func NewPageView(layout Layout) *PageViewBuilder {
	return &PageViewBuilder{
		layout: layout,
	}
}

// Title adds a title line (bold white).
func (b *PageViewBuilder) Title(title string) *PageViewBuilder {
	b.content.WriteString(RenderTitle(title))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// Subtitle adds a subtitle line (dim gray).
func (b *PageViewBuilder) Subtitle(subtitle string) *PageViewBuilder {
	b.content.WriteString(RenderDim(subtitle))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// Divider adds a full-width horizontal divider.
func (b *PageViewBuilder) Divider() *PageViewBuilder {
	b.content.WriteString(strings.Repeat("─", b.layout.InnerWidth))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// Spacing adds blank lines.
func (b *PageViewBuilder) Spacing(lines int) *PageViewBuilder {
	for i := 0; i < lines; i++ {
		b.content.WriteString("\n")
	}
	return b
}

// QueryInfo adds query/filter information line (accented yellow).
func (b *PageViewBuilder) QueryInfo(info string) *PageViewBuilder {
	b.content.WriteString(AccentStyle.Render(info))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// Text adds normal text content.
func (b *PageViewBuilder) Text(text string) *PageViewBuilder {
	b.content.WriteString(NormalStyle.Render(text))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// DimText adds dimmed text content.
func (b *PageViewBuilder) DimText(text string) *PageViewBuilder {
	b.content.WriteString(DimStyle.Render(text))
	b.content.WriteString("\n")
	b.hadContent = true
	return b
}

// CustomContent adds custom pre-rendered content.
// Use this for complex content that doesn't fit the builder pattern.
func (b *PageViewBuilder) CustomContent(content string) *PageViewBuilder {
	b.content.WriteString(content)
	b.hadContent = true
	return b
}

// Table adds a table with full-width selection highlighting.
// Should typically be added after QueryInfo and before Status.
func (b *PageViewBuilder) Table(t table.Model) *PageViewBuilder {
	// Add spacing before table if there's already content
	if b.hadContent {
		b.content.WriteString("\n")
	}
	b.content.WriteString(RenderTableWithSelection(t, b.layout))
	b.tableAdded = true
	b.hadContent = true
	return b
}

// Status adds a status message (if not empty).
// Typically added after the table or main content.
func (b *PageViewBuilder) Status(msg string) *PageViewBuilder {
	if msg != "" {
		// Add spacing before status if there's already content
		if b.hadContent {
			b.content.WriteString("\n")
		}
		b.content.WriteString(StatusMsgStyle.Render(msg))
		b.content.WriteString("\n")
		b.statusAdded = true
		b.hadContent = true
	}
	return b
}

// Error adds an error message.
func (b *PageViewBuilder) Error(err error) *PageViewBuilder {
	if err != nil {
		if b.hadContent {
			b.content.WriteString("\n")
		}
		b.content.WriteString(RenderError("Error: " + err.Error()))
		b.content.WriteString("\n")
		b.hadContent = true
	}
	return b
}

// Help sets the help text for the footer box.
// This should be called last before Build().
func (b *PageViewBuilder) Help(helpText string) *PageViewBuilder {
	b.helpText = helpText
	return b
}

// Build constructs the final view string with two-box layout.
// This must be called last to get the rendered output.
func (b *PageViewBuilder) Build() string {
	content := b.content.String()

	// Use the centralized TwoBoxView helper
	return TwoBoxView(content, b.helpText, b.layout)
}

// BuildContent builds just the content portion without the two-box layout.
// Use this if you need to do custom layout.
func (b *PageViewBuilder) BuildContent() string {
	return b.content.String()
}
