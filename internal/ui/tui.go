package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
	"github.com/thesavant42/gitsome-ng/internal/models"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Message types for async operations

// SwitchProjectMsg signals the TUI to exit and return to project selector
type SwitchProjectMsg struct{}

type fetchProgressMsg struct {
	fetched int
	page    int
}

type fetchCompleteMsg struct {
	owner   string
	name    string
	commits []models.Commit
	err     error
}

// User data fetch messages

type userQueryProgressMsg struct {
	login    string
	progress int
	total    int
}

type userQueryCompleteMsg struct {
	login string
	data  *models.UserData
	err   error
}

// Color palette for link groups - use from styles.go
// (keeping local reference for compatibility)
var linkColors = LinkColors

// Menu options
var menuOptions = []string{
	"Configure Highlight Domains",
	"Add Repository",
	"Query Tagged Users",
	"Switch Project",
	"Export Tab to Markdown",
	"Export Database Backup",
	"Export Project Report",
}

// gistFileEntry represents a flattened view of a gist file with parent gist info
type gistFileEntry struct {
	GistURL       string
	GistID        string
	FileName      string
	Language      string
	Size          int
	RevisionCount int
	UpdatedAt     string
	IsDivider     bool // true for divider rows (gist header)
	FileCount     int  // number of files in this gist (for divider display)
}

// TUIModel holds the state for the interactive table
type TUIModel struct {
	table        table.Model
	stats        []models.ContributorStats
	links        map[string]int  // email -> group_id
	tags         map[string]bool // email -> tagged
	pendingLinks []int           // row indices pending to be linked
	repoOwner    string
	repoName     string
	database     *db.DB
	tableType    string // "committers" or "authors"
	totalCommits int
	cached       bool
	quitting     bool
	helpVisible  bool

	// Layout state
	layout Layout

	// Menu state
	menuVisible bool
	menuCursor  int

	// Domain configuration state
	domainConfigVisible bool
	highlightDomains    map[string]int // domain -> color_index
	domainList          []string       // ordered list of domains for display
	domainCursor        int            // cursor for domain list
	domainInput         string         // text input buffer for new domain
	domainInputActive   bool           // whether text input is active

	// Multi-repo state
	repos            []models.RepoInfo // list of tracked repos
	currentRepoIndex int               // current repo page (-1 means combined stats page)
	showCombined     bool              // whether combined stats page is active

	// Add repo input state
	addRepoVisible     bool
	addRepoInput       string
	addRepoInputActive bool

	// API fetch state
	token           string           // GitHub API token
	fetchPromptRepo *models.RepoInfo // repo pending fetch confirmation
	fetchingRepo    *models.RepoInfo // repo currently being fetched
	fetchProgress   string           // progress message during fetch

	// Project switch state
	switchProject bool // true when user wants to switch to a different project

	// Export state
	dbPath        string // path to current database for backup export
	exportMessage string // message to show after export (success or error)

	// User query state
	queryingUsers      bool     // true when querying tagged users
	queryProgress      string   // progress message during query
	queryTotal         int      // total users to query
	queryCompleted     int      // users completed
	queryFailed        int      // users that failed
	queryLoginsToFetch []string // logins remaining to fetch

	// Progress bar state
	progressBar     progress.Model // animated progress bar component
	showProgress    bool           // flag to show/hide progress bar
	progressPercent float64        // current progress percentage (0.0 to 1.0)
	progressLabel   string         // descriptive label for current operation

	// User detail view state
	userDetailVisible   bool                    // showing user detail view
	selectedUserLogin   string                  // which user we're viewing
	selectedUserEmail   string                  // email from commits (if available)
	selectedUserName    string                  // name from commits (if available)
	selectedUserProfile models.UserProfile      // full profile from DB
	userRepos           []models.UserRepository // repos for selected user
	userGists           []models.UserGist       // gists for selected user
	userGistFiles       []gistFileEntry         // flattened gist files for display
	userDetailTab       int                     // 0 = repos, 1 = gists/files
	userDetailCursor    int                     // cursor position in detail view

	// Processed users cache - logins with fetched data show [!] instead of [x]
	processedLogins map[string]bool

	// Delete confirmation state
	deleteConfirmVisible bool
	deleteConfirmForm    *huh.Form
	deleteTargetIndex    int    // row index to delete
	deleteTargetType     string // "committer", "repo", "gist"

	// Edit form state
	editFormVisible bool
	editForm        *huh.Form
	editTargetIndex int    // row index being edited
	editTargetType  string // "committer", "profile"
	editLoginValue  string // temp storage for form values
	editNameValue   string
}

// isServiceAccount returns true if the user is a service account that cannot be scanned
// (either no login, or the GitHub web-flow service account)
func isServiceAccount(login, email string) bool {
	if login == "" {
		return true
	}
	if login == "web-flow" && email == "noreply@github.com" {
		return true
	}
	return false
}

// NewTUIModel creates a new interactive table model
func NewTUIModel(
	stats []models.ContributorStats,
	links map[string]int,
	tags map[string]bool,
	domains map[string]int,
	repoOwner, repoName string,
	database *db.DB,
	tableType string,
	totalCommits int,
	cached bool,
) TUIModel {
	// Use centralized column definitions from styles.go
	columns := TableColumns

	// Build processed logins cache - check which users have fetched data
	processedLogins := make(map[string]bool)
	if database != nil {
		for _, s := range stats {
			if s.GitHubLogin != "" {
				if hasData, _ := database.UserHasData(s.GitHubLogin); hasData {
					processedLogins[s.GitHubLogin] = true
				}
			}
		}
	}

	// Build rows
	rows := make([]table.Row, len(stats))
	for i, s := range stats {
		tagMark := "[ ]"
		// Service accounts (no login or web-flow) cannot be scanned, show [-]
		if isServiceAccount(s.GitHubLogin, s.Email) {
			tagMark = "[-]"
		} else if processedLogins[s.GitHubLogin] {
			// Has login and processed (data fetched) - shows [!]
			tagMark = "[!]"
		} else if tags[s.Email] {
			tagMark = "[x]"
		}

		login := s.GitHubLogin
		if login == "" {
			login = "-"
		}

		rows[i] = table.Row{
			tagMark,
			fmt.Sprintf("%d", i+1),
			s.Name,
			login,
			s.Email,
			fmt.Sprintf("%d", s.CommitCount),
			fmt.Sprintf("%.1f%%", s.Percentage),
		}
	}

	// Create table with styled headers, using centralized height from styles.go
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(TableHeight),
	)

	// Apply styles using centralized colors from styles.go
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorText).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorText)

	// Note: Cell foreground is set in renderTableWithLinks to avoid conflicts with link colors

	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)

	t.SetStyles(s)

	// Build ordered domain list from map
	domainList := make([]string, 0, len(domains))
	for domain := range domains {
		domainList = append(domainList, domain)
	}

	// Initialize progress bar with red from style guide
	layout := DefaultLayout()
	prog := progress.New(
		progress.WithSolidFill(string(ColorText)), // bright white fill (15)
		progress.WithWidth(layout.ContentWidth-4),
	)
	prog.EmptyColor = string(ColorTextDim) // gray background (241) for empty portion

	return TUIModel{
		table:            t,
		stats:            stats,
		links:            links,
		tags:             tags,
		highlightDomains: domains,
		domainList:       domainList,
		repoOwner:        repoOwner,
		repoName:         repoName,
		database:         database,
		tableType:        tableType,
		totalCommits:     totalCommits,
		cached:           cached,
		processedLogins:  processedLogins,
		layout:           DefaultLayout(),
		progressBar:      prog,
		showProgress:     false,
		progressPercent:  0.0,
		progressLabel:    "",
	}
}

// Init implements tea.Model
func (m TUIModel) Init() tea.Cmd {
	return tea.ClearScreen
}

// Update implements tea.Model
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	// Handle window resize
	case tea.WindowSizeMsg:
		m.layout = NewLayout(msg.Width)
		m.progressBar.Width = m.layout.ContentWidth - 4
		return m, nil

	// Handle progress bar frame messages for animation
	case progress.FrameMsg:
		progressModel, cmd := m.progressBar.Update(msg)
		m.progressBar = progressModel.(progress.Model)
		return m, cmd

	// Handle async fetch messages
	case fetchProgressMsg:
		m.fetchProgress = fmt.Sprintf("Fetching commits... %d fetched (page %d)", msg.fetched, msg.page)
		// Show progress bar during fetch (indeterminate progress since we don't know total)
		m.showProgress = true
		m.progressLabel = fmt.Sprintf("Fetching commits... page %d", msg.page)
		// Use a pulsing effect by setting a moderate progress value
		m.progressPercent = 0.5
		// SetPercent returns a command that triggers animation
		return m, m.progressBar.SetPercent(m.progressPercent)

	case fetchCompleteMsg:
		m.fetchingRepo = nil
		m.fetchProgress = ""

		// Hide progress bar on completion
		m.showProgress = false
		m.progressPercent = 0.0
		m.progressLabel = ""

		// Log API call to database
		if m.database != nil {
			endpoint := fmt.Sprintf("/repos/%s/%s/commits", msg.owner, msg.name)
			errorMsg := ""
			statusCode := 200
			if msg.err != nil {
				errorMsg = msg.err.Error()
				statusCode = 0
			}
			m.database.SaveAPILog("GET", endpoint, statusCode, errorMsg, 0, "", "")
		}

		if msg.err != nil {
			// Show error message to user
			m.exportMessage = fmt.Sprintf("Fetch failed: %v", msg.err)
			return m, nil
		}
		if len(msg.commits) == 0 {
			m.exportMessage = fmt.Sprintf("No commits found for %s/%s", msg.owner, msg.name)
		}
		// Store commits in database
		if m.database != nil && len(msg.commits) > 0 {
			records := make([]models.CommitRecord, len(msg.commits))
			for i, c := range msg.commits {
				records[i] = c.ToRecord(msg.owner, msg.name)
			}
			m.database.InsertCommits(records)
			// Ensure repo is tracked in database
			m.database.AddTrackedRepo(msg.owner, msg.name)
		}
		// Switch to the newly fetched repo
		for i, repo := range m.repos {
			if repo.Owner == msg.owner && repo.Name == msg.name {
				m.currentRepoIndex = i
				m.switchToRepo(i)
				break
			}
		}
		return m, nil

	case userQueryProgressMsg:
		m.queryProgress = fmt.Sprintf("Querying %s... (%d/%d)", msg.login, msg.progress, msg.total)
		// Show progress bar with actual percentage
		m.showProgress = true
		m.progressPercent = float64(msg.progress) / float64(msg.total)
		m.progressLabel = fmt.Sprintf("Querying users... %d/%d", msg.progress, msg.total)
		// SetPercent returns a command that triggers animation
		return m, m.progressBar.SetPercent(m.progressPercent)

	case userQueryCompleteMsg:
		m.queryCompleted++

		// Update progress bar
		m.progressPercent = float64(m.queryCompleted) / float64(m.queryTotal)
		m.progressLabel = fmt.Sprintf("Querying users... %d/%d", m.queryCompleted, m.queryTotal)
		m.queryProgress = fmt.Sprintf("Completed %d/%d users", m.queryCompleted, m.queryTotal)

		// Log API call to database
		if m.database != nil {
			endpoint := "https://api.github.com/graphql"
			errorMsg := ""
			statusCode := 200
			if msg.err != nil {
				errorMsg = msg.err.Error()
				statusCode = 0
				m.queryFailed++
			}
			m.database.SaveAPILog("POST", endpoint, statusCode, errorMsg, 0, "", msg.login)
		}

		if msg.err == nil && msg.data != nil && m.database != nil {
			// Save user data to database
			m.database.SaveUserProfile(msg.data.Profile)
			m.database.SaveUserRepositories(msg.data.Repositories)
			m.database.SaveUserGists(msg.data.Gists)
			// Mark this login as processed so it shows [!] instead of [x]
			if m.processedLogins == nil {
				m.processedLogins = make(map[string]bool)
			}
			m.processedLogins[msg.login] = true
		}
		// Fetch next user if any
		if len(m.queryLoginsToFetch) > 0 {
			login := m.queryLoginsToFetch[0]
			m.queryLoginsToFetch = m.queryLoginsToFetch[1:]
			// Update progress bar and start next query
			cmd := m.progressBar.SetPercent(m.progressPercent)
			return m, tea.Batch(cmd, m.startUserQuery(login, m.queryCompleted+1, m.queryTotal))
		}
		// All done - update rows to reflect new [!] indicators
		m.queryingUsers = false
		m.queryProgress = ""

		// Hide progress bar on completion
		m.showProgress = false
		m.progressPercent = 0.0
		m.progressLabel = ""

		m.updateRows()
		m.exportMessage = fmt.Sprintf("Queried %d users (%d succeeded, %d failed)", m.queryTotal, m.queryTotal-m.queryFailed, m.queryFailed)
		return m, nil

	}

	// Handle delete confirmation form (needs all msg types, not just KeyMsg)
	if m.deleteConfirmVisible && m.deleteConfirmForm != nil {
		form, cmd := m.deleteConfirmForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.deleteConfirmForm = f
		}

		switch m.deleteConfirmForm.State {
		case huh.StateCompleted:
			m.deleteConfirmVisible = false
			// Check if user confirmed
			confirm := m.deleteConfirmForm.GetBool("confirm")
			if confirm {
				m.executeDelete()
			}
			return m, nil
		case huh.StateAborted:
			m.deleteConfirmVisible = false
			return m, nil
		}
		return m, cmd
	}

	// Handle edit form (needs all msg types, not just KeyMsg)
	if m.editFormVisible && m.editForm != nil {
		form, cmd := m.editForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.editForm = f
		}

		switch m.editForm.State {
		case huh.StateCompleted:
			m.editFormVisible = false
			m.executeEdit()
			return m, nil
		case huh.StateAborted:
			m.editFormVisible = false
			return m, nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Clear export message on any key press
		if m.exportMessage != "" {
			m.exportMessage = ""
		}

		// Handle fetch prompt (Y/N)
		if m.fetchPromptRepo != nil {
			switch msg.String() {
			case "y", "Y":
				repo := m.fetchPromptRepo
				m.fetchPromptRepo = nil
				m.fetchingRepo = repo
				m.fetchProgress = "Starting fetch..."
				// Show progress bar from the start
				m.showProgress = true
				m.progressPercent = 0.5 // Indeterminate progress for fetching
				m.progressLabel = "Fetching commits..."
				// Trigger animation and start fetch
				cmd := m.progressBar.SetPercent(m.progressPercent)
				return m, tea.Batch(cmd, m.startFetch(repo.Owner, repo.Name))
			case "n", "N", "esc":
				m.fetchPromptRepo = nil
				return m, nil
			}
			return m, nil
		}

		// Handle add repo input mode
		if m.addRepoVisible && m.addRepoInputActive {
			return m.handleAddRepoInput(msg)
		}

		// Handle user detail view
		if m.userDetailVisible {
			return m.handleUserDetailView(msg)
		}

		// Handle domain config input mode
		if m.domainConfigVisible && m.domainInputActive {
			return m.handleDomainInput(msg)
		}

		// Handle domain config navigation
		if m.domainConfigVisible {
			return m.handleDomainConfig(msg)
		}

		// Handle menu navigation
		if m.menuVisible {
			return m.handleMenu(msg)
		}

		// Block input while fetching
		if m.fetchingRepo != nil || m.queryingUsers {
			return m, nil
		}

		// Main table mode
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil

		case "m", "M":
			m.menuVisible = true
			m.menuCursor = 0
			return m, nil

		case "left", "h":
			// Navigate to previous repo page
			if len(m.repos) > 0 {
				if m.showCombined {
					// Go from combined to last repo
					m.showCombined = false
					m.currentRepoIndex = len(m.repos) - 1
					m.switchToRepo(m.currentRepoIndex)
				} else if m.currentRepoIndex > 0 {
					m.currentRepoIndex--
					m.switchToRepo(m.currentRepoIndex)
				}
			}
			return m, nil

		case "right", "l":
			// Navigate to next repo page (but not when in link selection mode)
			if len(m.repos) > 0 && len(m.pendingLinks) == 0 {
				if !m.showCombined && m.currentRepoIndex < len(m.repos)-1 {
					m.currentRepoIndex++
					m.switchToRepo(m.currentRepoIndex)
				} else if !m.showCombined && m.currentRepoIndex == len(m.repos)-1 {
					// Go to combined stats page
					m.showCombined = true
					m.switchToCombined()
				}
			}
			return m, nil

		case "L":
			// Toggle row in/out of pending link selection
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				// Check if already in pending
				found := -1
				for i, idx := range m.pendingLinks {
					if idx == cursor {
						found = i
						break
					}
				}

				if found >= 0 {
					// Remove from pending
					m.pendingLinks = append(m.pendingLinks[:found], m.pendingLinks[found+1:]...)
				} else {
					// Add to pending
					m.pendingLinks = append(m.pendingLinks, cursor)
				}
			}
			return m, nil

		case "esc":
			// Commit pending links
			if len(m.pendingLinks) > 1 {
				// Find existing group ID from any pending row, or create new
				var groupID int
				for _, idx := range m.pendingLinks {
					email := m.stats[idx].Email
					if existingGroup, ok := m.links[email]; ok {
						groupID = existingGroup
						break
					}
				}

				// If no existing group, get next ID
				if groupID == 0 {
					nextID, err := m.database.GetNextGroupID(m.repoOwner, m.repoName)
					if err == nil {
						groupID = nextID
					} else {
						groupID = 1
					}
				}

				// Save all pending rows to this group
				for _, idx := range m.pendingLinks {
					email := m.stats[idx].Email
					m.links[email] = groupID
					if m.database != nil {
						m.database.SaveLink(m.repoOwner, m.repoName, groupID, email)
					}
				}
			}
			// Clear pending
			m.pendingLinks = nil
			return m, nil

		case "t", "T":
			// Toggle tag / clear processed status
			// Tags ALL rows with same GitHub login
			// Service accounts cannot be scanned (show [-])
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				email := m.stats[cursor].Email
				login := m.stats[cursor].GitHubLogin

				// Service accounts cannot be scanned, do nothing
				if isServiceAccount(login, email) {
					return m, nil
				}

				// If user is processed [!], pressing T clears their data for re-scan
				if m.processedLogins[login] {
					// Clear processed status and user data
					delete(m.processedLogins, login)
					if m.database != nil {
						m.database.DeleteUserRepositories(login)
						m.database.DeleteUserGists(login)
					}
					// Tag all rows with same login so they can be re-scanned
					for _, s := range m.stats {
						if s.GitHubLogin == login {
							m.tags[s.Email] = true
							if m.database != nil {
								m.database.SaveTag(m.repoOwner, m.repoName, s.Email)
							}
						}
					}
				} else if m.tags[email] {
					// Currently tagged [x] -> untag ALL rows with same login
					for _, s := range m.stats {
						if s.GitHubLogin == login {
							delete(m.tags, s.Email)
							if m.database != nil {
								m.database.RemoveTag(m.repoOwner, m.repoName, s.Email)
							}
						}
					}
				} else {
					// Currently untagged [ ] -> tag ALL rows with same login
					for _, s := range m.stats {
						if s.GitHubLogin == login {
							m.tags[s.Email] = true
							if m.database != nil {
								m.database.SaveTag(m.repoOwner, m.repoName, s.Email)
							}
						}
					}
				}
				// Update rows to reflect tag change
				m.updateRows()
			}
			return m, nil

		case "u", "U":
			// Unlink current row
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				email := m.stats[cursor].Email
				delete(m.links, email)
				if m.database != nil {
					m.database.RemoveLink(m.repoOwner, m.repoName, email)
				}
			}
			return m, nil

		case "enter":
			// View user detail if user has a GitHub login and data exists
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				s := m.stats[cursor]
				if s.GitHubLogin != "" && m.database != nil {
					// Check if user has data
					hasData, _ := m.database.UserHasData(s.GitHubLogin)
					if hasData {
						m.showUserDetail(s.GitHubLogin, s.Name, s.Email)
					}
				}
			}
			return m, nil

		case "d", "D", "delete":
			// Delete selected committer row with confirmation
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				s := m.stats[cursor]
				m.deleteTargetIndex = cursor
				m.deleteTargetType = "committer"

				m.deleteConfirmForm = huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Key("confirm").
							Title(fmt.Sprintf("Delete committer '%s' (%s)?", s.Name, s.Email)).
							Description("This will remove the committer from the database.").
							Affirmative("Yes, delete").
							Negative("Cancel"),
					),
				).WithTheme(NewAppTheme())

				m.deleteConfirmVisible = true
				return m, m.deleteConfirmForm.Init()
			}
			return m, nil

		case "e", "E":
			// Edit selected committer row
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				s := m.stats[cursor]
				m.editTargetIndex = cursor
				m.editTargetType = "committer"
				m.editLoginValue = s.GitHubLogin
				m.editNameValue = s.Name

				m.editForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Key("login").
							Title("GitHub Login").
							Description("GitHub username (leave empty to unlink)").
							Value(&m.editLoginValue),
						huh.NewInput().
							Key("name").
							Title("Display Name").
							Value(&m.editNameValue),
					),
				).WithTheme(NewAppTheme())

				m.editFormVisible = true
				return m, m.editForm.Init()
			}
			return m, nil
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// handleMenu handles key events when the menu is visible
func (m TUIModel) handleMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.menuVisible = false
		return m, nil

	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
		return m, nil

	case "down", "j":
		if m.menuCursor < len(menuOptions)-1 {
			m.menuCursor++
		}
		return m, nil

	case "enter":
		// Handle menu selection
		switch m.menuCursor {
		case 0: // Configure Highlight Domains
			m.menuVisible = false
			m.domainConfigVisible = true
			m.domainCursor = 0
			m.domainInput = ""
			m.domainInputActive = false
		case 1: // Add Repository
			m.menuVisible = false
			m.addRepoVisible = true
			m.addRepoInput = ""
			m.addRepoInputActive = true
		case 2: // Query Tagged Users
			m.menuVisible = false
			if m.database != nil && m.token != "" {
				// Get tagged users with GitHub logins
				taggedUsers, err := m.database.GetTaggedUsersWithLogins(m.repoOwner, m.repoName)
				if err != nil || len(taggedUsers) == 0 {
					m.exportMessage = "No tagged users with GitHub logins found"
				} else {
					// Filter out already processed users (those showing [!])
					var logins []string
					seenLogins := make(map[string]bool)
					for _, u := range taggedUsers {
						// Skip users who have already been processed
						if m.processedLogins != nil && m.processedLogins[u.GitHubLogin] {
							continue
						}
						// Skip duplicate logins (user may have multiple tagged emails)
						if seenLogins[u.GitHubLogin] {
							continue
						}
						seenLogins[u.GitHubLogin] = true
						logins = append(logins, u.GitHubLogin)
					}
					if len(logins) == 0 {
						m.exportMessage = "All tagged users have already been processed"
					} else {
						// Start querying users
						m.queryingUsers = true
						m.queryTotal = len(logins)
						m.queryCompleted = 0
						m.queryFailed = 0
						// Show progress bar from the start
						m.showProgress = true
						m.progressPercent = 0.0
						m.progressLabel = fmt.Sprintf("Querying users... 0/%d", len(logins))
						if len(logins) > 1 {
							m.queryLoginsToFetch = logins[1:]
						}
						// Trigger animation and start query
						cmd := m.progressBar.SetPercent(m.progressPercent)
						return m, tea.Batch(cmd, m.startUserQuery(logins[0], 1, m.queryTotal))
					}
				}
			} else if m.token == "" {
				m.exportMessage = "GitHub token required for user queries"
			}
		case 3: // Switch Project
			m.menuVisible = false
			m.switchProject = true
			return m, tea.Quit
		case 4: // Export Tab to Markdown
			m.menuVisible = false
			filename, err := ExportTabToMarkdown(m.stats, m.repoOwner, m.repoName, m.totalCommits, m.showCombined)
			if err != nil {
				m.exportMessage = fmt.Sprintf("Export failed: %v", err)
			} else {
				m.exportMessage = fmt.Sprintf("Exported to %s", filename)
			}
		case 5: // Export Database Backup
			m.menuVisible = false
			if m.dbPath != "" {
				filename, err := ExportDatabaseBackup(m.dbPath)
				if err != nil {
					m.exportMessage = fmt.Sprintf("Backup failed: %v", err)
				} else {
					m.exportMessage = fmt.Sprintf("Database backed up to %s", filename)
				}
			} else {
				m.exportMessage = "Database path not available"
			}
		case 6: // Export Project Report
			m.menuVisible = false
			if m.database != nil {
				filename, err := ExportProjectReport(m.database, m.dbPath)
				if err != nil {
					m.exportMessage = fmt.Sprintf("Project report failed: %v", err)
				} else {
					m.exportMessage = fmt.Sprintf("Project report exported to %s", filename)
				}
			} else {
				m.exportMessage = "Database not available"
			}
		}
		return m, nil
	}

	return m, nil
}

// handleDomainConfig handles key events in domain configuration screen
func (m TUIModel) handleDomainConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.domainConfigVisible = false
		return m, nil

	case "up", "k":
		if m.domainCursor > 0 {
			m.domainCursor--
		}
		return m, nil

	case "down", "j":
		if m.domainCursor < len(m.domainList)-1 {
			m.domainCursor++
		}
		return m, nil

	case "a", "A":
		// Activate input mode to add new domain
		m.domainInputActive = true
		m.domainInput = ""
		return m, nil

	case "d", "D", "delete", "backspace":
		// Delete selected domain
		if len(m.domainList) > 0 && m.domainCursor < len(m.domainList) {
			domain := m.domainList[m.domainCursor]
			delete(m.highlightDomains, domain)
			m.domainList = append(m.domainList[:m.domainCursor], m.domainList[m.domainCursor+1:]...)
			if m.database != nil {
				m.database.RemoveDomain(domain)
			}
			// Adjust cursor if needed
			if m.domainCursor >= len(m.domainList) && m.domainCursor > 0 {
				m.domainCursor--
			}
		}
		return m, nil
	}

	return m, nil
}

// handleDomainInput handles text input for adding new domains
func (m TUIModel) handleDomainInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.domainInputActive = false
		m.domainInput = ""
		return m, nil

	case "enter":
		// Add the domain if input is not empty
		domain := strings.TrimSpace(m.domainInput)
		if domain != "" && m.highlightDomains[domain] == 0 {
			// Get next color index
			colorIndex := 0
			if m.database != nil {
				nextIdx, err := m.database.GetNextDomainColorIndex()
				if err == nil {
					colorIndex = nextIdx
				}
			} else {
				// Find max color index in current map
				for _, idx := range m.highlightDomains {
					if idx >= colorIndex {
						colorIndex = idx + 1
					}
				}
			}

			m.highlightDomains[domain] = colorIndex
			m.domainList = append(m.domainList, domain)
			if m.database != nil {
				m.database.SaveDomain(domain, colorIndex)
			}
		}
		m.domainInputActive = false
		m.domainInput = ""
		return m, nil

	case "backspace":
		if len(m.domainInput) > 0 {
			m.domainInput = m.domainInput[:len(m.domainInput)-1]
		}
		return m, nil

	default:
		// Add character to input (filter to printable chars)
		if len(msg.String()) == 1 {
			m.domainInput += msg.String()
		}
		return m, nil
	}
}

// handleAddRepoInput handles text input for adding a new repository
func (m TUIModel) handleAddRepoInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addRepoInputActive = false
		m.addRepoVisible = false
		m.addRepoInput = ""
		return m, nil

	case "enter":
		// Parse and add the repository
		input := strings.TrimSpace(m.addRepoInput)
		if input != "" {
			parts := strings.Split(input, "/")
			if len(parts) == 2 {
				owner := strings.TrimSpace(parts[0])
				name := strings.TrimSpace(parts[1])
				if owner != "" && name != "" && m.database != nil {
					// Before adding new repo, ensure current repo is in the list
					if len(m.repos) == 0 {
						m.repos = append(m.repos, models.RepoInfo{Owner: m.repoOwner, Name: m.repoName})
						m.currentRepoIndex = 0
						// Also track the current repo in database
						m.database.AddTrackedRepo(m.repoOwner, m.repoName)
					}
					// Add to tracked repos
					m.database.AddTrackedRepo(owner, name)
					// Add to local list
					newRepo := models.RepoInfo{Owner: owner, Name: name}
					m.repos = append(m.repos, newRepo)

					// Check if repo has cached commits
					hasCached, _ := m.database.HasCachedCommits(owner, name)
					if !hasCached && m.token != "" {
						// Show fetch prompt
						m.fetchPromptRepo = &newRepo
					}
				}
			}
		}
		m.addRepoInputActive = false
		m.addRepoVisible = false
		m.addRepoInput = ""
		return m, nil

	case "backspace":
		if len(m.addRepoInput) > 0 {
			m.addRepoInput = m.addRepoInput[:len(m.addRepoInput)-1]
		}
		return m, nil

	default:
		// Add character to input (filter to printable chars)
		if len(msg.String()) == 1 {
			m.addRepoInput += msg.String()
		}
		return m, nil
	}
}

// switchToRepo loads data for the specified repo index
func (m *TUIModel) switchToRepo(index int) {
	if index < 0 || index >= len(m.repos) || m.database == nil {
		return
	}

	repo := m.repos[index]
	m.repoOwner = repo.Owner
	m.repoName = repo.Name

	// Load stats
	stats, total, err := m.database.GetCommitterStats(repo.Owner, repo.Name)
	if err != nil {
		stats = []models.ContributorStats{}
		total = 0
	}
	m.stats = stats
	m.totalCommits = total

	// Load links
	links, err := m.database.GetLinks(repo.Owner, repo.Name)
	if err != nil {
		links = make(map[string]int)
	}
	m.links = links

	// Load tags
	tags, err := m.database.GetTags(repo.Owner, repo.Name)
	if err != nil {
		tags = make(map[string]bool)
	}
	m.tags = tags

	// Load domains (global - shared across all repos)
	domains, err := m.database.GetDomains()
	if err != nil {
		domains = make(map[string]int)
	}
	m.highlightDomains = domains
	m.domainList = make([]string, 0, len(domains))
	for domain := range domains {
		m.domainList = append(m.domainList, domain)
	}

	// Clear pending links
	m.pendingLinks = nil

	// Rebuild table
	m.rebuildTable()
}

// switchToCombined loads combined stats across all repos
func (m *TUIModel) switchToCombined() {
	if m.database == nil {
		return
	}

	m.repoOwner = "Combined"
	m.repoName = "All Repos"

	// Load combined stats
	stats, total, err := m.database.GetCombinedCommitterStats()
	if err != nil {
		stats = []models.ContributorStats{}
		total = 0
	}
	m.stats = stats
	m.totalCommits = total

	// Clear repo-specific data for combined view
	m.links = make(map[string]int)
	m.tags = make(map[string]bool)
	m.pendingLinks = nil

	// Load global highlight domains
	domains, err := m.database.GetDomains()
	if err != nil {
		domains = make(map[string]int)
	}
	m.highlightDomains = domains
	m.domainList = make([]string, 0, len(domains))
	for domain := range domains {
		m.domainList = append(m.domainList, domain)
	}

	// Rebuild table
	m.rebuildTable()
}

// rebuildTable recreates the table with current stats
func (m *TUIModel) rebuildTable() {
	// Use centralized column definitions from styles.go
	columns := TableColumns

	// Rebuild processed logins cache
	m.processedLogins = make(map[string]bool)
	if m.database != nil {
		for _, s := range m.stats {
			if s.GitHubLogin != "" {
				if hasData, _ := m.database.UserHasData(s.GitHubLogin); hasData {
					m.processedLogins[s.GitHubLogin] = true
				}
			}
		}
	}

	rows := make([]table.Row, len(m.stats))
	for i, s := range m.stats {
		tagMark := "[ ]"
		// Service accounts (no login or web-flow) cannot be scanned, show [-]
		if isServiceAccount(s.GitHubLogin, s.Email) {
			tagMark = "[-]"
		} else if m.processedLogins[s.GitHubLogin] {
			// Has login and processed (data fetched) - shows [!]
			tagMark = "[!]"
		} else if m.tags[s.Email] {
			tagMark = "[x]"
		}

		login := s.GitHubLogin
		if login == "" {
			login = "-"
		}

		rows[i] = table.Row{
			tagMark,
			fmt.Sprintf("%d", i+1),
			s.Name,
			login,
			s.Email,
			fmt.Sprintf("%d", s.CommitCount),
			fmt.Sprintf("%.1f%%", s.Percentage),
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(TableHeight),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorText).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorText)

	s.Selected = s.Selected.
		Foreground(ColorText).
		Background(ColorHighlight).
		Bold(true)

	t.SetStyles(s)
	m.table = t
}

// startFetch returns a tea.Cmd that fetches commits from the GitHub API
func (m *TUIModel) startFetch(owner, name string) tea.Cmd {
	return func() tea.Msg {
		if m.token == "" {
			return fetchCompleteMsg{owner: owner, name: name, err: fmt.Errorf("no GitHub token")}
		}

		var client *api.Client
		if m.dbPath != "" {
			client = api.NewClientWithLogging(m.token, m.dbPath)
		} else {
			client = api.NewClient(m.token)
		}

		// Get latest SHA for incremental fetch
		var latestSHA string
		if m.database != nil {
			latestSHA, _ = m.database.GetLatestCommitSHA(owner, name)
		}

		commits, err := client.FetchCommits(owner, name, latestSHA, nil)
		if err != nil {
			return fetchCompleteMsg{owner: owner, name: name, err: err}
		}

		return fetchCompleteMsg{owner: owner, name: name, commits: commits}
	}
}

// startUserQuery returns a tea.Cmd that fetches user repos and gists
func (m *TUIModel) startUserQuery(login string, _, _ int) tea.Cmd {
	return func() tea.Msg {
		if m.token == "" {
			return userQueryCompleteMsg{login: login, err: fmt.Errorf("no GitHub token")}
		}

		var client *api.Client
		if m.dbPath != "" {
			client = api.NewClientWithLogging(m.token, m.dbPath)
		} else {
			client = api.NewClient(m.token)
		}

		userData, err := client.FetchUserReposAndGists(login)
		if err != nil {
			return userQueryCompleteMsg{login: login, err: err}
		}

		return userQueryCompleteMsg{login: login, data: userData}
	}
}

// handleUserDetailView handles key events in user detail view
func (m TUIModel) handleUserDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.userDetailVisible = false
		return m, nil

	case "tab", "right", "l":
		// Cycle forward: Profile(0) -> Repos(1) -> Gists(2) -> Profile(0)
		m.userDetailTab = (m.userDetailTab + 1) % 3
		m.userDetailCursor = 0
		return m, nil

	case "left", "h":
		// Cycle backward: Profile(0) <- Repos(1) <- Gists(2)
		m.userDetailTab = (m.userDetailTab + 2) % 3
		m.userDetailCursor = 0
		return m, nil

	case "up", "k":
		// Tab 0 is Profile (no list), Tab 1 is Repos, Tab 2 is Gists
		if m.userDetailTab == 2 {
			// Skip dividers when navigating gists
			for {
				if m.userDetailCursor <= 0 {
					break
				}
				m.userDetailCursor--
				if !m.userGistFiles[m.userDetailCursor].IsDivider {
					break
				}
			}
		} else if m.userDetailTab > 0 && m.userDetailCursor > 0 {
			m.userDetailCursor--
		}
		return m, nil

	case "down", "j":
		maxItems := 0
		switch m.userDetailTab {
		case 1:
			maxItems = len(m.userRepos)
		case 2:
			maxItems = len(m.userGistFiles)
			// Skip dividers when navigating gists
			for {
				if m.userDetailCursor >= maxItems-1 {
					break
				}
				m.userDetailCursor++
				if !m.userGistFiles[m.userDetailCursor].IsDivider {
					break
				}
			}
			return m, nil
		}
		if m.userDetailCursor < maxItems-1 {
			m.userDetailCursor++
		}
		return m, nil

	case "enter":
		// Tab 0: Profile - open GitHub profile
		if m.userDetailTab == 0 {
			if m.selectedUserLogin != "" {
				profileURL := fmt.Sprintf("https://github.com/%s", m.selectedUserLogin)
				openURL(profileURL)
			}
			return m, nil
		}
		// Tab 1: Repos - open selected repo in browser
		if m.userDetailTab == 1 && m.userDetailCursor < len(m.userRepos) {
			repo := m.userRepos[m.userDetailCursor]
			if repo.URL != "" {
				openURL(repo.URL)
			}
		}
		// Tab 2: Gists - open selected gist file in browser
		if m.userDetailTab == 2 && m.userDetailCursor < len(m.userGistFiles) {
			file := m.userGistFiles[m.userDetailCursor]
			if !file.IsDivider && file.GistURL != "" {
				// Construct file-specific URL: base_gist_url#file-filename-ext
				fileURL := file.GistURL
				if file.FileName != "" {
					// GitHub gist file anchor format: #file-name-with-dashes-extension
					anchor := strings.ReplaceAll(file.FileName, ".", "-")
					anchor = strings.ReplaceAll(anchor, "_", "-")
					anchor = strings.ToLower(anchor)
					fileURL = fileURL + "#file-" + anchor
				}
				openURL(fileURL)
			}
		}
		return m, nil

	case "p":
		// Open user's GitHub profile page
		if m.selectedUserLogin != "" {
			profileURL := fmt.Sprintf("https://github.com/%s", m.selectedUserLogin)
			openURL(profileURL)
		}
		return m, nil

	case "d", "D", "delete":
		// Delete selected repo or gist
		if m.userDetailTab == 1 && m.userDetailCursor < len(m.userRepos) {
			// Delete repo
			repo := m.userRepos[m.userDetailCursor]
			m.deleteTargetIndex = m.userDetailCursor
			m.deleteTargetType = "repo"

			m.deleteConfirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Key("confirm").
						Title(fmt.Sprintf("Delete repository '%s'?", repo.Name)).
						Description("This will remove the repository from the database.").
						Affirmative("Yes, delete").
						Negative("Cancel"),
				),
			).WithTheme(NewAppTheme())

			m.deleteConfirmVisible = true
			return m, m.deleteConfirmForm.Init()

		} else if m.userDetailTab == 2 && m.userDetailCursor < len(m.userGistFiles) {
			// Delete gist
			gf := m.userGistFiles[m.userDetailCursor]
			m.deleteTargetIndex = m.userDetailCursor
			m.deleteTargetType = "gist"

			// Different message for divider row vs file row
			var title, desc string
			if gf.IsDivider {
				title = fmt.Sprintf("Delete entire gist %s?", gf.GistID[:12])
				desc = fmt.Sprintf("This will remove all %d files from this gist.", gf.FileCount)
			} else {
				title = fmt.Sprintf("Delete gist '%s'?", gf.FileName)
				desc = "This will remove all files from this gist."
			}

			m.deleteConfirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Key("confirm").
						Title(title).
						Description(desc).
						Affirmative("Yes, delete").
						Negative("Cancel"),
				),
			).WithTheme(NewAppTheme())

			m.deleteConfirmVisible = true
			return m, m.deleteConfirmForm.Init()
		}
		return m, nil
	}

	return m, nil
}

// showUserDetail loads user data and shows the detail view
func (m *TUIModel) showUserDetail(login, name, email string) {
	if m.database == nil {
		return
	}

	// Store user info from commits
	m.selectedUserName = name
	m.selectedUserEmail = email

	// Load user profile from DB
	profile, err := m.database.GetUserProfile(login)
	if err != nil {
		profile = models.UserProfile{Login: login}
	}
	m.selectedUserProfile = profile

	// Load repos
	repos, err := m.database.GetUserRepositories(login)
	if err != nil {
		repos = []models.UserRepository{}
	}

	// Load gists
	gists, err := m.database.GetUserGists(login)
	if err != nil {
		gists = []models.UserGist{}
	}

	// Build flattened list of gist files with dividers
	var gistFiles []gistFileEntry
	for _, gist := range gists {
		files, err := m.database.GetGistFiles(gist.ID)
		fileCount := len(files)
		if err != nil || fileCount == 0 {
			fileCount = 1 // placeholder
		}

		// Insert divider row first
		gistFiles = append(gistFiles, gistFileEntry{
			GistURL:       gist.URL,
			GistID:        gist.ID,
			RevisionCount: gist.RevisionCount,
			UpdatedAt:     gist.UpdatedAt,
			IsDivider:     true,
			FileCount:     fileCount,
		})

		if err != nil || len(files) == 0 {
			// Show gist with no files as a placeholder entry
			gistFiles = append(gistFiles, gistFileEntry{
				GistURL:       gist.URL,
				GistID:        gist.ID,
				FileName:      "(no files)",
				Language:      "-",
				Size:          0,
				RevisionCount: gist.RevisionCount,
				UpdatedAt:     gist.UpdatedAt,
			})
		} else {
			for _, file := range files {
				gistFiles = append(gistFiles, gistFileEntry{
					GistURL:       gist.URL,
					GistID:        gist.ID,
					FileName:      file.Name,
					Language:      file.Language,
					Size:          file.Size,
					RevisionCount: gist.RevisionCount,
					UpdatedAt:     gist.UpdatedAt,
				})
			}
		}
	}

	m.selectedUserLogin = login
	m.userRepos = repos
	m.userGists = gists
	m.userGistFiles = gistFiles
	m.userDetailTab = 0
	m.userDetailCursor = 0
	m.userDetailVisible = true
}

// executeDelete performs the actual delete operation after confirmation
func (m *TUIModel) executeDelete() {
	switch m.deleteTargetType {
	case "committer":
		if m.deleteTargetIndex >= 0 && m.deleteTargetIndex < len(m.stats) {
			s := m.stats[m.deleteTargetIndex]

			// Remove from database
			if m.database != nil {
				m.database.DeleteCommitterByEmail(m.repoOwner, m.repoName, s.Email)
			}

			// Remove from in-memory stats
			m.stats = append(m.stats[:m.deleteTargetIndex], m.stats[m.deleteTargetIndex+1:]...)

			// Clean up related data
			delete(m.tags, s.Email)
			delete(m.links, s.Email)

			// Update table
			m.updateRows()
			m.exportMessage = fmt.Sprintf("Deleted committer: %s", s.Email)
		}

	case "repo":
		if m.deleteTargetIndex >= 0 && m.deleteTargetIndex < len(m.userRepos) {
			repo := m.userRepos[m.deleteTargetIndex]

			// Remove from database
			if m.database != nil {
				m.database.DeleteUserRepository(m.selectedUserLogin, repo.Name)
			}

			// Remove from in-memory list
			m.userRepos = append(m.userRepos[:m.deleteTargetIndex], m.userRepos[m.deleteTargetIndex+1:]...)

			// Adjust cursor if needed
			if m.userDetailCursor >= len(m.userRepos) && m.userDetailCursor > 0 {
				m.userDetailCursor--
			}
			m.exportMessage = fmt.Sprintf("Deleted repository: %s", repo.Name)
		}

	case "gist":
		if m.deleteTargetIndex >= 0 && m.deleteTargetIndex < len(m.userGistFiles) {
			gf := m.userGistFiles[m.deleteTargetIndex]

			// Remove from database
			if m.database != nil {
				m.database.DeleteUserGist(m.selectedUserLogin, gf.GistID)
			}

			// Rebuild gist files list (remove all files from this gist)
			var newFiles []gistFileEntry
			for _, f := range m.userGistFiles {
				if f.GistID != gf.GistID {
					newFiles = append(newFiles, f)
				}
			}
			m.userGistFiles = newFiles

			// Also remove from userGists
			var newGists []models.UserGist
			for _, g := range m.userGists {
				if g.ID != gf.GistID {
					newGists = append(newGists, g)
				}
			}
			m.userGists = newGists

			// Adjust cursor if needed
			if m.userDetailCursor >= len(m.userGistFiles) && m.userDetailCursor > 0 {
				m.userDetailCursor--
			}
			m.exportMessage = fmt.Sprintf("Deleted gist: %s", gf.GistID)
		}
	}
}

// executeEdit applies the edit form values
func (m *TUIModel) executeEdit() {
	switch m.editTargetType {
	case "committer":
		if m.editTargetIndex >= 0 && m.editTargetIndex < len(m.stats) {
			s := &m.stats[m.editTargetIndex]

			// Update GitHub login (in-memory and update commits table)
			oldLogin := s.GitHubLogin
			newLogin := strings.TrimSpace(m.editLoginValue)
			if newLogin != oldLogin {
				s.GitHubLogin = newLogin

				// Update github_committer_login in commits table
				if m.database != nil {
					m.database.UpdateCommitterLogin(m.repoOwner, m.repoName, s.Email, newLogin)
				}

				// Update processed status
				if oldLogin != "" {
					delete(m.processedLogins, oldLogin)
				}
				if newLogin != "" && m.database != nil {
					if hasData, _ := m.database.UserHasData(newLogin); hasData {
						m.processedLogins[newLogin] = true
					}
				}
			}

			// Update name (in-memory and update commits table)
			newName := strings.TrimSpace(m.editNameValue)
			if newName != s.Name {
				s.Name = newName
				if m.database != nil {
					m.database.UpdateCommitterName(m.repoOwner, m.repoName, s.Email, newName)
				}
			}

			m.updateRows()
			m.exportMessage = fmt.Sprintf("Updated committer: %s", s.Email)
		}

	case "profile":
		// Profile editing - update user profile fields
		if m.database != nil && m.selectedUserLogin != "" {
			profile := m.selectedUserProfile
			// Values would be set from the form
			// profile.TwitterUsername = m.editTwitterValue (etc.)
			m.database.SaveUserProfile(profile)
			m.exportMessage = fmt.Sprintf("Updated profile for: %s", m.selectedUserLogin)
		}
	}
}

// updateRows refreshes the table rows with current tag state
func (m *TUIModel) updateRows() {
	rows := make([]table.Row, len(m.stats))
	for i, s := range m.stats {
		tagMark := "[ ]"
		// Service accounts (no login or web-flow) cannot be scanned, show [-]
		if isServiceAccount(s.GitHubLogin, s.Email) {
			tagMark = "[-]"
		} else if m.processedLogins[s.GitHubLogin] {
			// Has login and processed (data fetched) - shows [!]
			tagMark = "[!]"
		} else if m.tags[s.Email] {
			tagMark = "[x]"
		}

		login := s.GitHubLogin
		if login == "" {
			login = "-"
		}

		rows[i] = table.Row{
			tagMark,
			fmt.Sprintf("%d", i+1),
			s.Name,
			login,
			s.Email,
			fmt.Sprintf("%d", s.CommitCount),
			fmt.Sprintf("%.1f%%", s.Percentage),
		}
	}
	m.table.SetRows(rows)
}

// View implements tea.Model
func (m TUIModel) View() string {
	if m.quitting {
		return ""
	}

	// Show delete confirmation form if visible
	if m.deleteConfirmVisible && m.deleteConfirmForm != nil {
		return m.renderFormOverlay(m.deleteConfirmForm.View(), "Delete Confirmation")
	}

	// Show edit form if visible
	if m.editFormVisible && m.editForm != nil {
		return m.renderFormOverlay(m.editForm.View(), "Edit Row")
	}

	// Show fetch prompt if pending
	if m.fetchPromptRepo != nil {
		return m.renderFetchPrompt()
	}

	// Show fetching status if in progress
	if m.fetchingRepo != nil {
		return m.renderFetchProgress()
	}

	// Show user query progress if in progress
	if m.queryingUsers {
		return m.renderQueryProgress()
	}

	// Show user detail view if visible
	if m.userDetailVisible {
		return m.renderUserDetail()
	}

	// Show add repo screen if visible
	if m.addRepoVisible {
		return m.renderAddRepo()
	}

	// Show domain config screen if visible
	if m.domainConfigVisible {
		return m.renderDomainConfig()
	}

	// Show menu if visible
	if m.menuVisible {
		return m.renderMenu()
	}

	var b strings.Builder

	// Add top margin to avoid terminal edge
	b.WriteString("\n")

	// Render page indicator if multiple repos
	if len(m.repos) > 0 {
		pageIndicator := m.renderPageIndicator()
		b.WriteString(pageIndicator)
		b.WriteString("\n")
	}

	// Render the table with border and custom row colors for links
	tableView := m.renderTableWithLinks()

	// Add border around table using centralized style with dynamic width
	borderedTable := BorderStyle.
		Width(m.layout.ViewportWidth).
		Render(tableView)
	b.WriteString(borderedTable)
	b.WriteString("\n")

	// Render progress bar if active
	if m.showProgress {
		// Render label above progress bar
		if m.progressLabel != "" {
			label := NormalStyle.Render(m.progressLabel)
			b.WriteString(" " + label)
			b.WriteString("\n")
		}

		// Render the progress bar
		progressView := m.progressBar.ViewAs(m.progressPercent)
		b.WriteString(" " + progressView)
		b.WriteString("\n")
	}

	// Footer: stats on left, help in center, total commits on right
	if m.helpVisible {
		help := `
Keyboard Controls:
  j/k or up/down Navigate rows
  left/right     Switch between repositories
  L              Select/deselect row for linking (yellow = pending)
  Esc            Commit selected rows as a link group
  U              Unlink current row from its group
  T              Toggle tag [ ]/[x], or clear [!] for re-scan
  M              Open menu
  ?              Toggle this help
  q              Quit

Tags: [ ]=untagged, [x]=tagged, [!]=scanned (press T to clear for re-scan)
`
		b.WriteString(NormalStyle.Render(help))
	} else {
		// Build footer with three sections

		leftPart := fmt.Sprintf("Total Committers: %d", len(m.stats))
		if len(m.pendingLinks) > 0 {
			leftPart += fmt.Sprintf(" [SELECTING: %d rows]", len(m.pendingLinks))
		}

		centerPart := "Press ? for help, q to quit"
		if m.cached {
			centerPart += " | " + AccentStyle.Render("CACHED")
		}
		if m.exportMessage != "" {
			centerPart = AccentStyle.Render(m.exportMessage)
		}

		rightPart := fmt.Sprintf("Total Commits: %d", m.totalCommits)

		// Calculate spacing to fit within ViewportWidth
		// The footer sits below the border, so it should match ViewportWidth
		// Subtract 1 for the leading space
		availableWidth := m.layout.ViewportWidth - 1
		usedWidth := len(leftPart) + len(centerPart) + len(rightPart)
		remainingSpace := availableWidth - usedWidth
		if remainingSpace < 4 {
			remainingSpace = 4
		}
		leftSpacing := remainingSpace / 2
		rightSpacing := remainingSpace - leftSpacing

		// Build footer line - add leading padding to align with border left edge
		footer := " " + StatsStyle.Render(leftPart) +
			strings.Repeat(" ", leftSpacing) +
			HintStyle.Render(centerPart) +
			strings.Repeat(" ", rightSpacing) +
			StatsStyle.Render(rightPart)

		b.WriteString(footer)
	}

	return b.String()
}

// renderPageIndicator renders the page indicator for multi-repo navigation
// The arrows are clamped to viewport edges, tabs scroll if they overflow
func (m TUIModel) renderPageIndicator() string {
	if len(m.repos) == 0 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText).
		Background(ColorHighlight).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 1)

	// Build all tab labels with their rendered widths
	type tabInfo struct {
		label    string
		rendered string
		width    int
		active   bool
	}

	var tabs []tabInfo
	for i, repo := range m.repos {
		label := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
		active := (i == m.currentRepoIndex && !m.showCombined)
		var rendered string
		if active {
			rendered = activeStyle.Render(label)
		} else {
			rendered = inactiveStyle.Render(label)
		}
		tabs = append(tabs, tabInfo{label, rendered, lipgloss.Width(rendered) + 1, active}) // +1 for space
	}

	// Add Combined tab
	combinedLabel := "Combined"
	var combinedRendered string
	if m.showCombined {
		combinedRendered = activeStyle.Render(combinedLabel)
	} else {
		combinedRendered = inactiveStyle.Render(combinedLabel)
	}
	tabs = append(tabs, tabInfo{combinedLabel, combinedRendered, lipgloss.Width(combinedRendered), m.showCombined})

	// Calculate available width for tabs (viewport - left padding - arrows - right padding)
	// Layout: "  < [tabs] >"  =>  2 + 2 + tabs + 2 = viewport
	availableWidth := m.layout.ViewportWidth - 6

	// Find which tabs to display, keeping active tab visible
	activeIdx := len(tabs) - 1 // Combined
	if !m.showCombined {
		activeIdx = m.currentRepoIndex
	}

	// Calculate total width of all tabs
	totalWidth := 0
	for _, t := range tabs {
		totalWidth += t.width
	}

	// Calculate visible range - default to showing all tabs
	var startIdx, endIdx int

	// If tabs don't fit, scroll to keep active tab visible
	if totalWidth > availableWidth {
		// Need to scroll - keep active tab visible, start from active and expand in both directions
		startIdx = activeIdx
		endIdx = activeIdx + 1
		currentWidth := tabs[activeIdx].width

		// Expand left and right alternately
		for {
			expanded := false
			// Try expanding left
			if startIdx > 0 && currentWidth+tabs[startIdx-1].width <= availableWidth {
				startIdx--
				currentWidth += tabs[startIdx].width
				expanded = true
			}
			// Try expanding right
			if endIdx < len(tabs) && currentWidth+tabs[endIdx].width <= availableWidth {
				currentWidth += tabs[endIdx].width
				endIdx++
				expanded = true
			}
			if !expanded {
				break
			}
		}
	} else {
		// All tabs fit, show them all
		startIdx = 0
		endIdx = len(tabs)
	}

	// Build the tab bar
	var tabsStr strings.Builder
	for i := startIdx; i < endIdx; i++ {
		tabsStr.WriteString(tabs[i].rendered)
		if i < endIdx-1 {
			tabsStr.WriteString(" ")
		}
	}

	// Calculate padding to push right arrow to edge
	tabsRendered := tabsStr.String()
	tabsWidth := lipgloss.Width(tabsRendered)
	rightPadding := availableWidth - tabsWidth
	if rightPadding < 0 {
		rightPadding = 0
	}

	// Build final string with arrows clamped to edges
	var b strings.Builder
	b.WriteString("  ") // Left padding to align with border

	// Left arrow - show if we can scroll left
	if startIdx > 0 {
		b.WriteString(ArrowStyle.Render("<"))
	} else if m.currentRepoIndex > 0 || m.showCombined {
		b.WriteString(ArrowStyle.Render("<"))
	} else {
		b.WriteString(" ")
	}
	b.WriteString(" ")

	// Tabs
	b.WriteString(tabsRendered)

	// Right padding
	b.WriteString(strings.Repeat(" ", rightPadding))

	// Right arrow - show if we can scroll right or navigate right
	if endIdx < len(tabs) || !m.showCombined {
		b.WriteString(ArrowStyle.Render(">"))
	} else {
		b.WriteString(" ")
	}

	return b.String()
}

// renderAddRepo renders the add repository input screen
func (m TUIModel) renderAddRepo() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Add Repository"))
	b.WriteString("\n\n")

	b.WriteString(HintStyle.Render("Enter repository in owner/repo format:"))
	b.WriteString("\n\n")

	b.WriteString(AccentStyle.Render("> "))
	b.WriteString(m.addRepoInput)
	b.WriteString("_")
	b.WriteString("\n\n")

	b.WriteString(HintStyle.Render("Enter: add repository | Esc: cancel"))

	// Add border with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderFormOverlay renders a huh form in a bordered overlay
func (m TUIModel) renderFormOverlay(formView string, title string) string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(formView)

	// Add border with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderFetchPrompt renders the fetch confirmation prompt
func (m TUIModel) renderFetchPrompt() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Fetch Commits"))
	b.WriteString("\n\n")

	b.WriteString("Repository ")
	b.WriteString(AccentStyle.Render(fmt.Sprintf("%s/%s", m.fetchPromptRepo.Owner, m.fetchPromptRepo.Name)))
	b.WriteString(" has no cached commits.\n\n")

	b.WriteString("Fetch commits from GitHub API?\n\n")

	b.WriteString(HintStyle.Render("Y: Fetch commits | N/Esc: Skip"))

	// Add border with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderQueryProgress renders the user query progress screen
func (m TUIModel) renderQueryProgress() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Querying Tagged Users"))
	b.WriteString("\n\n")

	b.WriteString(ProgressStyle.Render(m.queryProgress))
	b.WriteString("\n\n")

	// Render animated progress bar
	if m.showProgress {
		progressView := m.progressBar.ViewAs(m.progressPercent)
		b.WriteString(progressView)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("Completed: %d / %d", m.queryCompleted, m.queryTotal))
	if m.queryFailed > 0 {
		b.WriteString(fmt.Sprintf(" (Failed: %d)", m.queryFailed))
	}
	b.WriteString("\n")

	// Add border with width from layout
	borderStyle := BorderStyle.
		Width(m.layout.ViewportWidth).
		MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderUserDetail renders the user detail view with repos and gists
func (m TUIModel) renderUserDetail() string {
	var b strings.Builder

	// User label and profile tab (tab 0) - all on same line
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render("User: "))
	userInfo := m.selectedUserLogin
	if m.userDetailTab == 0 {
		b.WriteString(TabActiveStyle.Render(userInfo))
	} else {
		b.WriteString(TabInactiveStyle.Render(userInfo))
	}
	b.WriteString(" ")

	// Repos tab (tab 1)
	if m.userDetailTab == 1 {
		b.WriteString(TabActiveStyle.Render(fmt.Sprintf("Repos (%d)", len(m.userRepos))))
	} else {
		b.WriteString(TabInactiveStyle.Render(fmt.Sprintf("Repos (%d)", len(m.userRepos))))
	}
	b.WriteString(" ")

	// Gists tab (tab 2) - count dividers as gists (files are grouped under dividers)
	gistCount := 0
	for _, gf := range m.userGistFiles {
		if gf.IsDivider {
			gistCount++
		}
	}
	if m.userDetailTab == 2 {
		b.WriteString(TabActiveStyle.Render(fmt.Sprintf("Gists (%d)", gistCount)))
	} else {
		b.WriteString(TabInactiveStyle.Render(fmt.Sprintf("Gists (%d)", gistCount)))
	}
	b.WriteString("\n\n")

	// Content based on active tab
	if m.userDetailTab == 0 {
		// Profile tab - show user info from DB profile + commit data
		p := m.selectedUserProfile
		b.WriteString(NormalStyle.Render("GitHub Profile"))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", m.layout.InnerWidth))
		b.WriteString("\n\n")

		// Username (always show)
		b.WriteString(NormalStyle.Render("Username:   "))
		b.WriteString(AccentStyle.Render(m.selectedUserLogin))
		b.WriteString("\n")

		// Name - prefer profile, fallback to commit data
		displayName := p.Name
		if displayName == "" {
			displayName = m.selectedUserName
		}
		b.WriteString(NormalStyle.Render("Name:       "))
		if displayName != "" {
			b.WriteString(NormalStyle.Render(displayName))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Email - prefer profile, fallback to commit data
		displayEmail := p.Email
		if displayEmail == "" {
			displayEmail = m.selectedUserEmail
		}
		b.WriteString(NormalStyle.Render("Email:      "))
		if displayEmail != "" {
			b.WriteString(NormalStyle.Render(displayEmail))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Bio
		b.WriteString(NormalStyle.Render("Bio:        "))
		if p.Bio != "" {
			// Truncate long bios
			bio := p.Bio
			if len(bio) > 60 {
				bio = bio[:57] + "..."
			}
			b.WriteString(NormalStyle.Render(bio))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Company
		b.WriteString(NormalStyle.Render("Company:    "))
		if p.Company != "" {
			b.WriteString(NormalStyle.Render(p.Company))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Location
		b.WriteString(NormalStyle.Render("Location:   "))
		if p.Location != "" {
			b.WriteString(NormalStyle.Render(p.Location))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Website
		b.WriteString(NormalStyle.Render("Website:    "))
		if p.WebsiteURL != "" {
			b.WriteString(NormalStyle.Render(p.WebsiteURL))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Twitter
		b.WriteString(NormalStyle.Render("Twitter:    "))
		if p.TwitterUsername != "" {
			b.WriteString(NormalStyle.Render("@" + p.TwitterUsername))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Pronouns
		b.WriteString(NormalStyle.Render("Pronouns:   "))
		if p.Pronouns != "" {
			b.WriteString(NormalStyle.Render(p.Pronouns))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Followers/Following
		b.WriteString(NormalStyle.Render("Followers:  "))
		if p.FollowerCount > 0 || p.Login != "" {
			b.WriteString(NormalStyle.Render(fmt.Sprintf("%d", p.FollowerCount)))
			b.WriteString(NormalStyle.Render("  |  Following: "))
			b.WriteString(NormalStyle.Render(fmt.Sprintf("%d", p.FollowingCount)))
		} else {
			b.WriteString(NormalStyle.Render("-"))
		}
		b.WriteString("\n")

		// Social accounts
		if len(p.SocialAccounts) > 0 {
			b.WriteString(NormalStyle.Render("Socials:    "))
			for i, social := range p.SocialAccounts {
				if i > 0 {
					b.WriteString(NormalStyle.Render(", "))
				}
				b.WriteString(AccentStyle.Render(social.Provider + ": " + social.DisplayName))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(HintStyle.Render("Press Enter to open profile in browser"))
	} else if m.userDetailTab == 1 {
		// Repos tab
		if len(m.userRepos) == 0 {
			b.WriteString(HintStyle.Render("No repositories found."))
		} else {
			// Show header with Language column
			b.WriteString(NormalStyle.Render(fmt.Sprintf("%-26s %-10s %-6s %-6s %-7s %-10s", "Name", "Lang", "Stars", "Forks", "Commits", "Visibility")))
			b.WriteString("\n")
			b.WriteString(strings.Repeat("─", m.layout.InnerWidth))
			b.WriteString("\n")

			// Show repos (limited to 15 visible)
			startIdx := 0
			if m.userDetailCursor >= 15 {
				startIdx = m.userDetailCursor - 14
			}
			endIdx := startIdx + 15
			if endIdx > len(m.userRepos) {
				endIdx = len(m.userRepos)
			}

			for i := startIdx; i < endIdx; i++ {
				repo := m.userRepos[i]
				name := repo.Name
				if len(name) > 24 {
					name = name[:21] + "..."
				}
				lang := repo.PrimaryLanguage
				if lang == "" {
					lang = "-"
				}
				if len(lang) > 9 {
					lang = lang[:8] + "…"
				}
				line := fmt.Sprintf("%-26s %-10s %-6d %-6d %-7d %-10s", name, lang, repo.StargazerCount, repo.ForkCount, repo.CommitCount, repo.Visibility)
				if i == m.userDetailCursor {
					b.WriteString(SelectedStyle.Width(m.layout.InnerWidth).Render(line))
				} else {
					b.WriteString(NormalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(HintStyle.Render("Press Enter to open in browser"))
	} else if m.userDetailTab == 2 {
		// Gists tab - show files
		if len(m.userGistFiles) == 0 {
			b.WriteString(HintStyle.Render("No gist files found."))
		} else {
			// Show header with GUID column (indented to match file rows)
			b.WriteString(NormalStyle.Render(fmt.Sprintf("  %-26s %-10s %-7s %-4s %-10s %-32s", "Filename", "Language", "Size", "Rev", "Updated", "GUID")))
			b.WriteString("\n")
			b.WriteString(strings.Repeat("─", m.layout.InnerWidth))
			b.WriteString("\n")

			// Show files (limited to 15 visible)
			startIdx := 0
			if m.userDetailCursor >= 15 {
				startIdx = m.userDetailCursor - 14
			}
			endIdx := startIdx + 15
			if endIdx > len(m.userGistFiles) {
				endIdx = len(m.userGistFiles)
			}

			for i := startIdx; i < endIdx; i++ {
				file := m.userGistFiles[i]

				// Check if this is a divider row
				if file.IsDivider {
					// Show label on separate line
					filesWord := "file"
					if file.FileCount != 1 {
						filesWord = "files"
					}
					updated := file.UpdatedAt
					if len(updated) > 10 {
						updated = updated[:10]
					}
					label := fmt.Sprintf("  %d %s · %s", file.FileCount, filesWord, updated)
					b.WriteString(NormalStyle.Render(label))
					b.WriteString("\n")

					// Render solid white divider spanning full width
					dividerText := strings.Repeat("─", m.layout.InnerWidth)
					b.WriteString(dividerText)
					b.WriteString("\n")
					continue
				}

				name := file.FileName
				if len(name) > 26 {
					name = name[:23] + "..."
				}
				lang := file.Language
				if lang == "" {
					lang = "-"
				}
				if len(lang) > 8 {
					lang = lang[:8]
				}
				// Format size
				sizeStr := fmt.Sprintf("%dB", file.Size)
				if file.Size >= 1024 {
					sizeStr = fmt.Sprintf("%.1fKB", float64(file.Size)/1024)
				}
				// Format date (just show date part)
				updated := file.UpdatedAt
				if len(updated) > 10 {
					updated = updated[:10]
				}
				// Extract GUID from GistID (remove prefix if present)
				guid := file.GistID
				if len(guid) > 32 {
					guid = guid[len(guid)-32:]
				}
				line := fmt.Sprintf("  %-26s %-10s %-7s %-4d %-10s %-32s", name, lang, sizeStr, file.RevisionCount, updated, guid)
				if i == m.userDetailCursor {
					b.WriteString(SelectedStyle.Width(m.layout.InnerWidth).Render(line))
				} else {
					b.WriteString(NormalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(HintStyle.Render("Press Enter to open in browser"))
	}

	b.WriteString("\n")
	b.WriteString(HintStyle.Render("left/right: switch tabs | j/k: navigate | Enter: open in browser | p: profile | Esc: back"))

	// Add border with width from layout
	borderStyle := BorderStyle.
		Width(m.layout.ViewportWidth).
		MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderFetchProgress renders the fetch progress screen
func (m TUIModel) renderFetchProgress() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Fetching Commits"))
	b.WriteString("\n\n")

	b.WriteString("Repository: ")
	b.WriteString(AccentStyle.Render(fmt.Sprintf("%s/%s", m.fetchingRepo.Owner, m.fetchingRepo.Name)))
	b.WriteString("\n\n")

	b.WriteString(ProgressStyle.Render(m.fetchProgress))
	b.WriteString("\n\n")

	// Render animated progress bar
	if m.showProgress {
		progressView := m.progressBar.ViewAs(m.progressPercent)
		b.WriteString(progressView)
		b.WriteString("\n")
	}

	// Add border with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderMenu renders the menu overlay
func (m TUIModel) renderMenu() string {
	var b strings.Builder

	menuSelectedStyle := SelectedStyle.Padding(0, 1)
	menuNormalStyle := NormalStyle.Padding(0, 1)

	b.WriteString(TitleStyle.Render("Menu"))
	b.WriteString("\n\n")

	for i, option := range menuOptions {
		if i == m.menuCursor {
			b.WriteString(menuSelectedStyle.Render("> " + option))
		} else {
			b.WriteString(menuNormalStyle.Render("  " + option))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HintStyle.Render("Enter: select | Esc: close"))

	// Add border around menu with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderDomainConfig renders the domain configuration screen
func (m TUIModel) renderDomainConfig() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Configure Highlight Domains"))
	b.WriteString("\n\n")

	if len(m.domainList) == 0 {
		b.WriteString(HintStyle.Render("No domains configured. Press A to add one."))
		b.WriteString("\n")
	} else {
		for i, domain := range m.domainList {
			domainStyle := AccentStyle

			prefix := "  "
			if i == m.domainCursor {
				prefix = "> "
				domainStyle = SelectedStyle
			}

			b.WriteString(prefix)
			b.WriteString(domainStyle.Render(domain))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Show input field if active
	if m.domainInputActive {
		b.WriteString(AccentStyle.Render("Add domain: "))
		b.WriteString(m.domainInput)
		b.WriteString("_")
		b.WriteString("\n\n")
		b.WriteString(HintStyle.Render("Enter: save | Esc: cancel"))
	} else {
		b.WriteString(HintStyle.Render("A: add domain | D: delete selected | Esc: back"))
	}

	// Add border around config screen with width from layout
	borderStyle := BorderStyle.Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// extractDomain extracts the domain part from an email address
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return strings.ToLower(parts[1])
	}
	return ""
}

// extractEmailFromRow extracts the email address from a rendered table row
// Uses regex to find email pattern, which is more robust than fixed-width parsing
func extractEmailFromRow(line string) string {
	// Look for email pattern: something@something.something
	// The email is in column 5 of the table
	for _, part := range strings.Fields(line) {
		if strings.Contains(part, "@") && strings.Contains(part, ".") {
			// Clean up any trailing/leading non-email characters
			email := strings.TrimSpace(part)
			return email
		}
	}
	return ""
}

// renderTableWithLinks renders the table with colored rows for linked groups
// STYLE GUIDE: All dividers and selectors use m.layout.InnerWidth for edge-to-edge rendering
func (m TUIModel) renderTableWithLinks() string {
	// Get the base table view
	baseView := m.table.View()

	// Build pending emails set for quick lookup (convert indices to emails)
	pendingEmails := make(map[string]bool)
	for _, idx := range m.pendingLinks {
		if idx >= 0 && idx < len(m.stats) {
			pendingEmails[m.stats[idx].Email] = true
		}
	}

	// Split into lines and colorize based on row content
	lines := strings.Split(baseView, "\n")
	result := make([]string, len(lines))

	// Header row + border line = 2 lines before data
	headerLines := 2

	// Get current cursor position to identify selected row
	cursor := m.table.Cursor()

	for i, line := range lines {
		// Replace the table's built-in divider (line 1) with full-width white divider
		// STYLE GUIDE: Dividers use InnerWidth (not ViewportWidth) to fit inside border
		if i == 1 {
			result[i] = strings.Repeat("─", m.layout.InnerWidth)
			continue
		}
		// Keep header row (line 0)
		if i < headerLines {
			result[i] = line
			continue
		}

		// Calculate data row index (subtract header lines)
		dataRowIndex := i - headerLines

		// Check if this is the selected row
		// STYLE GUIDE: Selectors use .Width(InnerWidth) for edge-to-edge rendering
		if dataRowIndex == cursor {
			result[i] = SelectedStyle.Width(m.layout.InnerWidth).Render(line)
			continue
		}

		// Extract email from the row content to identify the row
		email := extractEmailFromRow(line)
		if email == "" {
			// No email found, probably not a data row
			result[i] = line
			continue
		}

		// Check if pending (yellow background)
		if pendingEmails[email] {
			style := lipgloss.NewStyle().
				Foreground(ColorBlack).
				Background(ColorAccentDim)
			result[i] = style.Render(line)
			continue
		}

		// Check if linked (colored text) - links have highest priority
		if groupID, ok := m.links[email]; ok {
			colorIdx := (groupID - 1) % len(linkColors)
			color := linkColors[colorIdx]
			style := lipgloss.NewStyle().Foreground(color)
			result[i] = style.Render(line)
			continue
		}

		// Check if email domain matches a highlight domain
		domain := extractDomain(email)
		if _, ok := m.highlightDomains[domain]; ok {
			style := lipgloss.NewStyle().Foreground(ColorAccent)
			result[i] = style.Render(line)
			continue
		}

		// Apply bright white to non-highlighted rows
		style := lipgloss.NewStyle().Foreground(ColorText)
		result[i] = style.Render(line)
	}

	return strings.Join(result, "\n")
}

// RunInteractiveTable starts the interactive table TUI for a single repo
func RunInteractiveTable(
	stats []models.ContributorStats,
	repoOwner, repoName string,
	database *db.DB,
	tableType string,
	totalCommits int,
	cached bool,
	token string,
) error {
	// Load existing links and tags
	links, err := database.GetLinks(repoOwner, repoName)
	if err != nil {
		links = make(map[string]int)
	}

	tags, err := database.GetTags(repoOwner, repoName)
	if err != nil {
		tags = make(map[string]bool)
	}

	// Load existing highlight domains (global - shared across all repos)
	domains, err := database.GetDomains()
	if err != nil {
		domains = make(map[string]int)
	}

	model := NewTUIModel(stats, links, tags, domains, repoOwner, repoName, database, tableType, totalCommits, cached)
	model.token = token
	p := tea.NewProgram(model, tea.WithAltScreen())

	_, err = p.Run()
	return err
}

// RunMultiRepoTUI starts the interactive table TUI with multiple repositories
func RunMultiRepoTUI(
	repos []models.RepoInfo,
	database *db.DB,
	tableType string,
	token string,
	dbPath string,
) (bool, error) {
	if len(repos) == 0 {
		return false, fmt.Errorf("no repositories to display")
	}

	// Start with the first repo
	firstRepo := repos[0]

	// Load stats for first repo
	stats, totalCommits, err := database.GetCommitterStats(firstRepo.Owner, firstRepo.Name)
	if err != nil {
		stats = []models.ContributorStats{}
		totalCommits = 0
	}

	// Load existing links and tags
	links, err := database.GetLinks(firstRepo.Owner, firstRepo.Name)
	if err != nil {
		links = make(map[string]int)
	}

	tags, err := database.GetTags(firstRepo.Owner, firstRepo.Name)
	if err != nil {
		tags = make(map[string]bool)
	}

	// Load existing highlight domains (global - shared across all repos)
	domains, err := database.GetDomains()
	if err != nil {
		domains = make(map[string]int)
	}

	model := NewTUIModel(stats, links, tags, domains, firstRepo.Owner, firstRepo.Name, database, tableType, totalCommits, false)

	// Set up multi-repo state
	model.repos = repos
	model.currentRepoIndex = 0
	model.showCombined = false
	model.token = token
	model.dbPath = dbPath

	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	// Check if user wants to switch projects
	if m, ok := finalModel.(TUIModel); ok && m.switchProject {
		return true, nil
	}
	return false, nil
}

// openURL opens a URL in the default browser (cross-platform)
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
