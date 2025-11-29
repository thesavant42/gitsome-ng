package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
	"github.com/thesavant42/gitsome-ng/internal/models"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
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

	// User detail view state
	userDetailVisible bool                    // showing user detail view
	selectedUserLogin string                  // which user we're viewing
	userRepos         []models.UserRepository // repos for selected user
	userGists         []models.UserGist       // gists for selected user
	userGistFiles     []gistFileEntry         // flattened gist files for display
	userDetailTab     int                     // 0 = repos, 1 = gists/files
	userDetailCursor  int                     // cursor position in detail view

	// Processed users cache - logins with fetched data show [!] instead of [x]
	processedLogins map[string]bool
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
		// Check if user has been processed (data fetched) - shows [!]
		if s.GitHubLogin != "" && processedLogins[s.GitHubLogin] {
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
		BorderForeground(ColorBorder).
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
		return m, nil

	// Handle async fetch messages
	case fetchProgressMsg:
		m.fetchProgress = fmt.Sprintf("Fetching commits... %d fetched (page %d)", msg.fetched, msg.page)
		return m, nil

	case fetchCompleteMsg:
		m.fetchingRepo = nil
		m.fetchProgress = ""
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
		return m, nil

	case userQueryCompleteMsg:
		m.queryCompleted++
		if msg.err != nil {
			m.queryFailed++
		} else if msg.data != nil && m.database != nil {
			// Save user data to database
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
			return m, m.startUserQuery(login, m.queryCompleted+1, m.queryTotal)
		}
		// All done - update rows to reflect new [!] indicators
		m.queryingUsers = false
		m.queryProgress = ""
		m.updateRows()
		m.exportMessage = fmt.Sprintf("Queried %d users (%d succeeded, %d failed)", m.queryTotal, m.queryTotal-m.queryFailed, m.queryFailed)
		return m, nil

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
				return m, m.startFetch(repo.Owner, repo.Name)
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
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.stats) {
				email := m.stats[cursor].Email
				login := m.stats[cursor].GitHubLogin

				// If user is processed [!], pressing T clears their data for re-scan
				if login != "" && m.processedLogins[login] {
					// Clear processed status and user data
					delete(m.processedLogins, login)
					if m.database != nil {
						m.database.DeleteUserRepositories(login)
						m.database.DeleteUserGists(login)
					}
					// Keep them tagged [x] so they can be re-scanned
					m.tags[email] = true
					if m.database != nil {
						m.database.SaveTag(m.repoOwner, m.repoName, email)
					}
				} else if m.tags[email] {
					// Currently tagged [x] -> untag [ ]
					delete(m.tags, email)
					if m.database != nil {
						m.database.RemoveTag(m.repoOwner, m.repoName, email)
					}
				} else {
					// Currently untagged [ ] -> tag [x]
					m.tags[email] = true
					if m.database != nil {
						m.database.SaveTag(m.repoOwner, m.repoName, email)
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
				login := m.stats[cursor].GitHubLogin
				if login != "" && m.database != nil {
					// Check if user has data
					hasData, _ := m.database.UserHasData(login)
					if hasData {
						m.showUserDetail(login)
					}
				}
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
					for _, u := range taggedUsers {
						// Skip users who have already been processed
						if m.processedLogins != nil && m.processedLogins[u.GitHubLogin] {
							continue
						}
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
						if len(logins) > 1 {
							m.queryLoginsToFetch = logins[1:]
						}
						return m, m.startUserQuery(logins[0], 1, m.queryTotal)
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
		// Check if user has been processed (data fetched) - shows [!]
		if s.GitHubLogin != "" && m.processedLogins[s.GitHubLogin] {
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
		BorderForeground(ColorBorder).
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

		client := api.NewClient(m.token)

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

		client := api.NewClient(m.token)
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

	case "tab", "left", "right", "h", "l":
		// Toggle between repos and gists tabs
		m.userDetailTab = (m.userDetailTab + 1) % 2
		m.userDetailCursor = 0
		return m, nil

	case "up", "k":
		if m.userDetailCursor > 0 {
			m.userDetailCursor--
		}
		return m, nil

	case "down", "j":
		maxItems := 0
		if m.userDetailTab == 0 {
			maxItems = len(m.userRepos)
		} else {
			maxItems = len(m.userGistFiles)
		}
		if m.userDetailCursor < maxItems-1 {
			m.userDetailCursor++
		}
		return m, nil

	case "enter":
		// Open selected repo or gist in browser
		if m.userDetailTab == 0 && m.userDetailCursor < len(m.userRepos) {
			repo := m.userRepos[m.userDetailCursor]
			if repo.URL != "" {
				openURL(repo.URL)
			}
		} else if m.userDetailTab == 1 && m.userDetailCursor < len(m.userGistFiles) {
			file := m.userGistFiles[m.userDetailCursor]
			if file.GistURL != "" {
				openURL(file.GistURL)
			}
		}
		return m, nil
	}

	return m, nil
}

// showUserDetail loads user data and shows the detail view
func (m *TUIModel) showUserDetail(login string) {
	if m.database == nil {
		return
	}

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

	// Build flattened list of gist files
	var gistFiles []gistFileEntry
	for _, gist := range gists {
		files, err := m.database.GetGistFiles(gist.ID)
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

// updateRows refreshes the table rows with current tag state
func (m *TUIModel) updateRows() {
	rows := make([]table.Row, len(m.stats))
	for i, s := range m.stats {
		tagMark := "[ ]"
		// Check if user has been processed (data fetched) - shows [!]
		if s.GitHubLogin != "" && m.processedLogins[s.GitHubLogin] {
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

	// Calculate visible range
	startIdx := 0
	endIdx := len(tabs)

	// Calculate total width of all tabs
	totalWidth := 0
	for _, t := range tabs {
		totalWidth += t.width
	}

	// If tabs fit, show all
	if totalWidth <= availableWidth {
		startIdx = 0
		endIdx = len(tabs)
	} else {
		// Need to scroll - keep active tab visible
		// Start from active and expand in both directions
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
	borderStyle := BorderStyle.Padding(1, 2).Width(m.layout.ViewportWidth).MarginTop(1)

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
	borderStyle := BorderStyle.Padding(1, 2).Width(m.layout.ViewportWidth).MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderQueryProgress renders the user query progress screen
func (m TUIModel) renderQueryProgress() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Querying Tagged Users"))
	b.WriteString("\n\n")

	b.WriteString(ProgressStyle.Render(m.queryProgress))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Completed: %d / %d", m.queryCompleted, m.queryTotal))
	if m.queryFailed > 0 {
		b.WriteString(fmt.Sprintf(" (Failed: %d)", m.queryFailed))
	}
	b.WriteString("\n")

	// Add border with width from layout
	borderStyle := BorderStyle.
		Padding(1, 2).
		Width(m.layout.ViewportWidth).
		MarginTop(1)

	return borderStyle.Render(b.String())
}

// renderUserDetail renders the user detail view with repos and gists
func (m TUIModel) renderUserDetail() string {
	var b strings.Builder

	// Header with tabs on same line
	b.WriteString(TitleStyle.Render("User: "))
	b.WriteString(AccentStyle.Render(m.selectedUserLogin))
	b.WriteString("    ") // spacing between username and tabs
	if m.userDetailTab == 0 {
		b.WriteString(TabActiveStyle.Render(fmt.Sprintf("Repos (%d)", len(m.userRepos))))
	} else {
		b.WriteString(TabInactiveStyle.Render(fmt.Sprintf("Repos (%d)", len(m.userRepos))))
	}
	b.WriteString(" ")
	if m.userDetailTab == 1 {
		b.WriteString(TabActiveStyle.Render(fmt.Sprintf("Files (%d)", len(m.userGistFiles))))
	} else {
		b.WriteString(TabInactiveStyle.Render(fmt.Sprintf("Files (%d)", len(m.userGistFiles))))
	}
	b.WriteString("\n\n")

	// Content based on active tab
	if m.userDetailTab == 0 {
		// Repos tab
		if len(m.userRepos) == 0 {
			b.WriteString(HintStyle.Render("No repositories found."))
		} else {
			// Show header
			b.WriteString(NormalStyle.Render(fmt.Sprintf("%-30s %-8s %-8s %-9s %-10s", "Name", "Stars", "Forks", "Commits", "Visibility")))
			b.WriteString("\n")
			b.WriteString(NormalStyle.Render(strings.Repeat("-", 70)))
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
				if len(name) > 28 {
					name = name[:25] + "..."
				}
				line := fmt.Sprintf("%-30s %-8d %-8d %-9d %-10s", name, repo.StargazerCount, repo.ForkCount, repo.CommitCount, repo.Visibility)
				if i == m.userDetailCursor {
					b.WriteString(SelectedStyle.Render(line))
				} else {
					b.WriteString(NormalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
	} else {
		// Gists tab - show files
		if len(m.userGistFiles) == 0 {
			b.WriteString(HintStyle.Render("No gist files found."))
		} else {
			// Show header
			b.WriteString(NormalStyle.Render(fmt.Sprintf("%-30s %-12s %-8s %-5s %-12s", "Filename", "Language", "Size", "Revs", "Updated")))
			b.WriteString("\n")
			b.WriteString(NormalStyle.Render(strings.Repeat("-", 70)))
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
				name := file.FileName
				if len(name) > 28 {
					name = name[:25] + "..."
				}
				lang := file.Language
				if lang == "" {
					lang = "-"
				}
				if len(lang) > 10 {
					lang = lang[:10]
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
				line := fmt.Sprintf("%-30s %-12s %-8s %-5d %-12s", name, lang, sizeStr, file.RevisionCount, updated)
				if i == m.userDetailCursor {
					b.WriteString(SelectedStyle.Render(line))
				} else {
					b.WriteString(NormalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(HintStyle.Render("left/right: switch tabs | j/k: navigate | Enter: open in browser | Esc: back"))

	// Add border with width from layout
	borderStyle := BorderStyle.
		Padding(1, 2).
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
	b.WriteString("\n")

	// Add border with width from layout
	borderStyle := BorderStyle.Padding(1, 2).Width(m.layout.ViewportWidth).MarginTop(1)

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
	borderStyle := BorderStyle.Padding(1, 2).Width(m.layout.ViewportWidth).MarginTop(1)

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
	borderStyle := BorderStyle.Padding(1, 2).Width(m.layout.ViewportWidth).MarginTop(1)

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

	for i, line := range lines {
		// Skip header lines
		if i < headerLines {
			result[i] = line
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
