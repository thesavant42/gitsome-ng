# yolosint

GitHub commit analyzer with an interactive TUI for tracking committers across multiple repositories.

## Features

- Track multiple GitHub repositories in isolated projects
- View commit statistics with committer breakdown
- Interactive terminal UI with keyboard navigation
- Highlight email domains with custom colors
- Tag committers and query their GitHub repos/gists
- User detail view showing repositories and gists
- Open repos/gists directly in browser
- Export data to Markdown with clickable GitHub profile links
- Export comprehensive project reports
- Database backup/export functionality
- Project management (create, switch, backup projects)

## Installation

### From Source (go install)

```bash
go install github.com/thesavant42/gitsome-ng/cmd/yolosint@latest
```

### Build Locally

```bash
git clone https://github.com/thesavant42/gitsome-ng.git
cd gitsome-ng
go build -o yolosint ./cmd/yolosint
```

## Authentication

Requires `GITHUB_TOKEN` environment variable set to a GitHub Personal Access Token.

## Usage

### Interactive Mode (Recommended)

```bash
yolosint
```

This launches the project selector where you can open existing projects, create new ones, or exit.

### Command Line Flags

```bash
yolosint --db myproject.db    # Use specific database
yolosint --add-repo owner/repo # Add repository
yolosint --list-repos          # List tracked repos
yolosint owner/repo            # Legacy single-repo mode
```

## Keyboard Controls

| Key | Action |
|-----|--------|
| Up/Down | Navigate rows |
| Left/Right | Switch repositories / tabs |
| Enter | Open menu / detail view / browser |
| t/T | Tag/untag committer |
| Tab | Switch tabs in detail view |
| Esc | Return from detail view |
| q | Quit |

## Menu Options

1. **Configure Highlight Domains** - Color-code email domains
2. **Add Repository** - Track a new GitHub repository
3. **Query Tagged Users** - Fetch repos/gists for tagged users
4. **Switch Project** - Open a different database
5. **Export Tab to Markdown** - Save current view as Markdown
6. **Export Database Backup** - Copy database file
7. **Export Project Report** - Full project report with all data

## Tagging and Scanning

1. Navigate to a committer and press `t` to tag them `[x]`
2. Open menu and select "Query Tagged Users"
3. Tagged users are scanned and marked `[!]` when complete
4. Press Enter on a scanned user to view their repos/gists
5. Press Enter on a repo/gist row to open it in browser

## Data Storage

Project databases (`.db` files) are created in your current working directory.

## Legacy Python Version

The original Python implementation is available in the `legacy_python/` directory.

## License

See LICENSE file for details.
