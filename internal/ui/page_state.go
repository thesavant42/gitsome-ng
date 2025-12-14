package ui

import (
	"time"
)

// page_state.go provides shared state management for all TUI pages.
// Embed PageState in your page models to get consistent state handling.

// PageState contains common state that all pages need.
// Embed this in your page model to avoid duplicating these fields.
type PageState struct {
	Layout       Layout
	ViewMode     string
	StatusMsg    string
	StatusExpiry time.Time
	Quitting     bool
	ReturnToMain bool
}

// NewPageState creates a new PageState with the given layout.
func NewPageState(layout Layout) PageState {
	return PageState{
		Layout:   layout,
		ViewMode: "",
		Quitting: false,
	}
}

// SetStatus sets a status message that will expire after the given duration.
// If duration is 0, the status message will not expire.
func (p *PageState) SetStatus(msg string, duration time.Duration) {
	p.StatusMsg = msg
	if duration > 0 {
		p.StatusExpiry = time.Now().Add(duration)
	} else {
		p.StatusExpiry = time.Time{} // Zero time = no expiry
	}
}

// ClearExpiredStatus clears the status message if it has expired.
// Call this in your Update() function to automatically clear old messages.
func (p *PageState) ClearExpiredStatus() {
	if !p.StatusExpiry.IsZero() && time.Now().After(p.StatusExpiry) {
		p.StatusMsg = ""
		p.StatusExpiry = time.Time{}
	}
}

// HasStatus returns true if there is a non-empty status message.
func (p *PageState) HasStatus() bool {
	return p.StatusMsg != ""
}

// UpdateLayout updates the layout and returns true if it changed.
// Use this in your WindowSizeMsg handler.
func (p *PageState) UpdateLayout(width, height int) bool {
	newLayout := NewLayout(width, height)
	if newLayout.ViewportWidth != p.Layout.ViewportWidth ||
		newLayout.ViewportHeight != p.Layout.ViewportHeight {
		p.Layout = newLayout
		return true
	}
	return false
}
