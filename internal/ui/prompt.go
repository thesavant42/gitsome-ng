package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
)

// sanitizeInput removes null bytes and other invisible control characters from input
func sanitizeInput(s string) string {
	// Remove null bytes and other control characters (except whitespace)
	result := strings.Map(func(r rune) rune {
		// Keep printable characters and normal whitespace (space, tab, newline)
		if r == 0 || (r < 32 && r != '\t' && r != '\n' && r != '\r') {
			return -1 // Remove the character
		}
		return r
	}, s)
	return result
}

// PromptForRepo prompts the user to enter a repository in owner/repo format
func PromptForRepo() (owner, repo string, err error) {
	var repoInput string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter GitHub Repository").
				Description("Format: owner/repo (e.g., raspberrypi/utils)").
				Placeholder("owner/repo").
				Value(&repoInput).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("repository cannot be empty")
					}
					parts := strings.Split(s, "/")
					if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
						return fmt.Errorf("invalid format: use owner/repo")
					}
					return nil
				}),
		),
	)

	err = form.Run()
	if err != nil {
		return "", "", fmt.Errorf("prompt cancelled: %w", err)
	}

	// Sanitize input to remove null bytes and control characters
	repoInput = sanitizeInput(repoInput)
	parts := strings.Split(strings.TrimSpace(repoInput), "/")
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

// PromptForGitHubToken optionally prompts for a GitHub token
func PromptForGitHubToken() (string, error) {
	var token string
	var useToken bool

	// First ask if they want to use a token
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Use GitHub Token?").
				Description("A token increases rate limits from 60 to 5000 requests/hour").
				Affirmative("Yes").
				Negative("No").
				Value(&useToken),
		),
	)

	if err := confirmForm.Run(); err != nil {
		return "", nil // Continue without token on cancel
	}

	if !useToken {
		return "", nil
	}

	// Prompt for the token
	tokenForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GitHub Personal Access Token").
				Description("Token will not be stored").
				EchoMode(huh.EchoModePassword).
				Value(&token),
		),
	)

	if err := tokenForm.Run(); err != nil {
		return "", nil // Continue without token on cancel
	}

	return token, nil
}

// ConfirmFetch asks user to confirm fetching commits
func ConfirmFetch(owner, repo string) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Fetch commits from %s/%s?", owner, repo)).
				Description("This will download all commits and store them in SQLite").
				Affirmative("Yes, fetch commits").
				Negative("Cancel").
				Value(&confirm),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// PromptForExportWithTimeout asks user if they want to export results to markdown
// Returns false if timeout expires with no response
func PromptForExportWithTimeout(timeoutSeconds int) bool {
	var export bool
	done := make(chan bool, 1)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Export to Markdown? (auto-skip in %ds)", timeoutSeconds)).
				Description("Save the results as a markdown document").
				Affirmative("Yes").
				Negative("No").
				Value(&export),
		),
	)

	go func() {
		if err := form.Run(); err != nil {
			done <- false
			return
		}
		done <- export
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		fmt.Println("\nExport prompt timed out, skipping...")
		return false
	}
}

// PromptForUpdate asks user if they want to check for updates
func PromptForUpdate() (bool, error) {
	var update bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Check for updates?").
				Description("Fetch new commits from GitHub (N uses cached data)").
				Affirmative("Yes").
				Negative("No").
				Value(&update),
		),
	)

	if err := form.Run(); err != nil {
		return false, nil // Default to no update on cancel
	}

	return update, nil
}

// PromptForFilename asks user for an export filename
func PromptForFilename(defaultName string) (string, error) {
	var filename string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Export Filename").
				Description("Enter the filename for the markdown export").
				Placeholder(defaultName).
				Value(&filename).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return nil // Will use default
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("prompt cancelled: %w", err)
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = defaultName
	}

	// Add .md extension if not present
	if !strings.HasSuffix(strings.ToLower(filename), ".md") {
		filename = filename + ".md"
	}

	return filename, nil
}

