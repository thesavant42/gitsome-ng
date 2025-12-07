package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
	"github.com/thesavant42/gitsome-ng/internal/api"
	"github.com/thesavant42/gitsome-ng/internal/db"
	"github.com/thesavant42/gitsome-ng/internal/models"
	"github.com/thesavant42/gitsome-ng/internal/ui"
)

const (
	defaultDBPath = "charming-commits.db"
)

func main() {
	// Show splash screen on startup
	ui.ShowSplash()

	// Load .env file if it exists (silently ignore if not found)
	_ = godotenv.Load()

	// Parse command line flags
	repoFlag := flag.String("repo", "", "GitHub repository in owner/repo format (legacy single-repo mode)")
	fileFlag := flag.String("file", "", "Load commits from local JSON file instead of API")
	dbPath := flag.String("db", "", "Path to SQLite database file (bypasses project selector)")
	tokenFlag := flag.String("token", "", "GitHub personal access token (optional)")
	addRepoFlag := flag.String("add-repo", "", "Add a repository to tracking (owner/repo format)")
	listReposFlag := flag.Bool("list-repos", false, "List all tracked repositories")
	flag.Parse()

	// Also accept repo as positional argument
	if *repoFlag == "" && flag.NArg() > 0 {
		*repoFlag = flag.Arg(0)
	}

	// Resolve token early for TUI use
	token := *tokenFlag
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	// Determine database path
	var selectedDBPath string

	if *dbPath != "" {
		// Explicit --db flag bypasses project selector
		selectedDBPath = *dbPath
	} else if *repoFlag != "" || *fileFlag != "" || *addRepoFlag != "" || *listReposFlag {
		// Legacy mode with specific flags - use default database
		selectedDBPath = defaultDBPath
	} else {
		// No specific flags - show project selector
		result, err := ui.RunProjectSelector()
		if err != nil {
			ui.PrintError(fmt.Sprintf("Project selector failed: %v", err))
			os.Exit(1)
		}

		switch result.Action {
		case "exit":
			return
		case "open":
			selectedDBPath = result.ProjectPath
		case "create":
			selectedDBPath = result.ProjectPath
			fmt.Println()
			ui.PrintSuccess(fmt.Sprintf("Creating new project: %s", selectedDBPath))
		}
	}

	// Initialize database
	database, err := db.New(selectedDBPath)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Failed to initialize database: %v", err))
		os.Exit(1)
	}
	defer database.Close()

	// Handle --list-repos flag
	if *listReposFlag {
		repos, err := database.GetTrackedRepos()
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get tracked repos: %v", err))
			os.Exit(1)
		}
		if len(repos) == 0 {
			fmt.Println("No repositories are being tracked.")
			fmt.Println("Use --add-repo owner/repo to add one.")
		} else {
			fmt.Println("Tracked repositories:")
			for i, r := range repos {
				fmt.Printf("  %d. %s/%s\n", i+1, r.Owner, r.Name)
			}
		}
		return
	}

	// Handle --add-repo flag
	if *addRepoFlag != "" {
		owner, repo, err := api.ParseRepoString(*addRepoFlag)
		if err != nil {
			ui.PrintError(err.Error())
			os.Exit(1)
		}
		if err := database.AddTrackedRepo(owner, repo); err != nil {
			ui.PrintError(fmt.Sprintf("Failed to add repo: %v", err))
			os.Exit(1)
		}
		ui.PrintSuccess(fmt.Sprintf("Added %s/%s to tracked repositories", owner, repo))

		// Optionally fetch commits for the new repo
		fmt.Println()
		fmt.Println("Fetching commits for the new repository...")
		fetchAndStoreCommits(tokenFlag, owner, repo, database)
		return
	}

	// Main application loop - supports switching projects
	for {
		var owner, repo string

		// Get repository - from flag or prompt
		if *repoFlag != "" {
			owner, repo, err = api.ParseRepoString(*repoFlag)
			if err != nil {
				ui.PrintError(err.Error())
				os.Exit(1)
			}
		} else if *fileFlag == "" {
			// Check if we have tracked repos - if so, launch multi-repo TUI
			trackedRepos, err := database.GetTrackedRepos()
			if err == nil && len(trackedRepos) > 0 {
				// Clear screen before launching main TUI (fixes ghost flash from alt-screen transition)
				fmt.Print("\033[H\033[2J")

				// Launch multi-repo TUI
				result, err := ui.RunMultiRepoTUI(trackedRepos, database, "Committers", token, selectedDBPath)
				if err != nil {
					ui.PrintError(fmt.Sprintf("Interactive mode failed: %v", err))
					os.Exit(1)
				}

				// If user wants to launch Docker Hub search
				if result.LaunchDockerSearch {
					if err := ui.RunDockerHubSearch(log.Default(), database, result.DockerSearchQuery); err != nil {
						ui.PrintError(fmt.Sprintf("Docker Hub search failed: %v", err))
					}
					continue // Return to main TUI after search
				}

				// If user wants to browse a specific Docker Hub repository
				if result.LaunchBrowseDockerRepo {
					if err := ui.RunBrowseDockerHubRepo(log.Default(), database); err != nil {
						ui.PrintError(fmt.Sprintf("Browse Docker Hub repo failed: %v", err))
					}
					continue // Return to main TUI after browsing
				}

				// If user wants to browse cached layers
				if result.LaunchCachedLayers {
					if err := ui.RunCachedLayersBrowser(database); err != nil {
						ui.PrintError(fmt.Sprintf("Cached layers browser failed: %v", err))
					}
					continue // Return to main TUI after browsing
				}

				// If user wants to search cached layers
				if result.LaunchSearchCachedLayers {
					if err := ui.RunSearchCachedLayers(database); err != nil {
						ui.PrintError(fmt.Sprintf("Search cached layers failed: %v", err))
					}
					continue // Return to main TUI after search
				}

				// If user wants to launch Wayback Machine browser
				if result.LaunchWayback {
					if err := ui.RunWaybackBrowser(log.Default(), database); err != nil {
						ui.PrintError(fmt.Sprintf("Wayback browser failed: %v", err))
					}
					continue // Return to main TUI after browsing
				}

				// If user wants to browse cached Wayback CDX records
				if result.LaunchWaybackCache {
					if err := ui.RunWaybackCacheBrowser(log.Default(), database); err != nil {
						ui.PrintError(fmt.Sprintf("Wayback cache browser failed: %v", err))
					}
					continue // Return to main TUI after browsing
				}

				// If user wants to switch projects, close current db and show selector
				if result.SwitchProject {
					database.Close()

					// Clear screen before showing project selector
					fmt.Print("\033[H\033[2J")

					result, err := ui.RunProjectSelector()
					if err != nil {
						ui.PrintError(fmt.Sprintf("Project selector failed: %v", err))
						os.Exit(1)
					}

					if result.Action == "exit" {
						return
					}

					selectedDBPath = result.ProjectPath
					if result.Action == "create" {
						fmt.Println()
						ui.PrintSuccess(fmt.Sprintf("Creating new project: %s", selectedDBPath))
					}

					// Reopen database with new path
					database, err = db.New(selectedDBPath)
					if err != nil {
						ui.PrintError(fmt.Sprintf("Failed to initialize database: %v", err))
						os.Exit(1)
					}
					continue // Loop back to show the new project
				}
				return
			}

			// No tracked repos - prompt for a new one
			owner, repo, err = ui.PromptForRepo()
			if err != nil {
				ui.PrintError(err.Error())
				os.Exit(1)
			}
		}

		// Check if cache exists for this repo
		hasCached, _ := database.HasCachedCommits(owner, repo)

		// Handle data loading
		if *fileFlag != "" {
			// Load from file
			commits, fileOwner, fileRepo, err := loadFromFile(*fileFlag)
			if err != nil {
				ui.PrintError(err.Error())
				os.Exit(1)
			}
			owner = fileOwner
			repo = fileRepo
			ui.PrintSuccess(fmt.Sprintf("Loaded %d commits from %s", len(commits), *fileFlag))

			// Ensure repo is tracked before storing commits
			if err := database.AddTrackedRepo(owner, repo); err != nil {
				ui.PrintError(fmt.Sprintf("Failed to add tracked repo: %v", err))
				os.Exit(1)
			}

			// Store in database
			records := make([]models.CommitRecord, len(commits))
			for i, c := range commits {
				records[i] = c.ToRecord(owner, repo)
			}
			if err := database.InsertCommits(records); err != nil {
				ui.PrintError(fmt.Sprintf("Failed to store commits: %v", err))
				os.Exit(1)
			}
			ui.PrintSuccess(fmt.Sprintf("Stored %d commits in database", len(records)))
		}

		var usedCache bool
		if *fileFlag == "" {
			if hasCached {
				// Cache exists - always prompt to check for updates (default: No)
				shouldUpdate, _ := ui.PromptForUpdate()
				if !shouldUpdate {
					usedCache = true
				} else {
					fetchAndStoreCommits(tokenFlag, owner, repo, database)
				}
			} else {
				// No cache - must fetch from API
				fetchAndStoreCommits(tokenFlag, owner, repo, database)
			}
		}

		// Generate statistics
		fmt.Println()

		committerStats, totalCommits, err := database.GetCommitterStats(owner, repo)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get committer stats: %v", err))
			os.Exit(1)
		}

		// Launch interactive TUI
		if err := ui.RunInteractiveTable(committerStats, owner, repo, database, "Committers", totalCommits, usedCache, token); err != nil {
			ui.PrintError(fmt.Sprintf("Interactive mode failed: %v", err))
			os.Exit(1)
		}

		// Single-repo mode doesn't support project switching, so exit
		break
	}
}

// fetchAndStoreCommits fetches commits from GitHub API and stores them
func fetchAndStoreCommits(tokenFlag *string, owner, repo string, database *db.DB) {
	token := *tokenFlag
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	if token == "" {
		ui.PrintError("GITHUB_TOKEN not set. Please set it in .env file or environment.")
		os.Exit(1)
	}
	ui.PrintSuccess("GitHub token found")

	// Check for existing commits to enable incremental fetch
	latestSHA, _ := database.GetLatestCommitSHA(owner, repo)

	client := api.NewClient(token)
	fmt.Println()
	commits, err := client.FetchCommits(owner, repo, latestSHA, ui.PrintProgress)
	if err != nil {
		fmt.Println()
		ui.PrintError(fmt.Sprintf("Failed to fetch commits: %v", err))
		os.Exit(1)
	}
	fmt.Println()

	if len(commits) == 0 {
		ui.PrintSuccess("No new commits to fetch")
	} else {
		ui.PrintSuccess(fmt.Sprintf("Fetched %d commits from %s/%s", len(commits), owner, repo))

		// Ensure repo is tracked before storing commits
		if err := database.AddTrackedRepo(owner, repo); err != nil {
			ui.PrintError(fmt.Sprintf("Failed to add tracked repo: %v", err))
			os.Exit(1)
		}

		// Store in database
		records := make([]models.CommitRecord, len(commits))
		for i, c := range commits {
			records[i] = c.ToRecord(owner, repo)
		}
		if err := database.InsertCommits(records); err != nil {
			ui.PrintError(fmt.Sprintf("Failed to store commits: %v", err))
			os.Exit(1)
		}
		ui.PrintSuccess(fmt.Sprintf("Stored %d commits in database", len(records)))
	}
}

// loadFromFile loads commits from a JSON file and infers owner/repo from filename
func loadFromFile(filePath string) ([]models.Commit, string, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read file: %w", err)
	}

	commits, err := api.ParseCommitsFromJSON(data)
	if err != nil {
		return nil, "", "", err
	}

	// Try to infer owner/repo from filename (e.g., "raspberry-pi-commits.json")
	base := filepath.Base(filePath)
	name := base[:len(base)-len(filepath.Ext(base))]

	// Use filename as repo identifier
	owner := "local"
	repo := name

	return commits, owner, repo, nil
}
