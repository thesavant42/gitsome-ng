# yolosint

GitHub commit analyzer with an interactive TUI for tracking committers across multiple repositories.

## Features

- Track multiple GitHub repositories in isolated projects
- View commit statistics with committer breakdown
- Interactive terminal UI with keyboard navigation
- Highlight email domains with custom colors
- Add links and tags to committers
- Export data to Markdown with clickable GitHub profile links
- Database backup/export functionality
- Project management (create, switch, backup projects)

## Installation

### From Source (go install)

```bash
go install github.com/thesavant42/gitsome-ng/cmd/yolosint@latest
```

After installation, `yolosint` will be available in your PATH.

### Build Locally

```bash
git clone https://github.com/thesavant42/gitsome-ng.git
cd gitsome-ng
go build -o yolosint ./cmd/yolosint
```

## Authentication

yolosint requires a GitHub Personal Access Token (PAT) to fetch commit data.

### Setting up your GitHub Token

**On Windows (PowerShell):**
```powershell
$env:GITHUB_TOKEN="your_token_here"
```

**On Windows (Command Prompt):**
```cmd
set GITHUB_TOKEN=your_token_here
```

**On Linux/macOS:**
```bash
export GITHUB_TOKEN="your_token_here"
```

Alternatively, create a `.env` file in your working directory:
```
GITHUB_TOKEN=your_token_here
```

## Usage

### Interactive Mode (Recommended)

Simply run:
```bash
yolosint
```

This launches the project selector where you can:
- Open an existing project database
- Create a new project
- Exit

### Command Line Flags

```bash
# Use a specific database file
yolosint --db myproject.db

# Add a repository to tracking
yolosint --add-repo owner/repo

# List tracked repositories
yolosint --list-repos

# Legacy single-repo mode
yolosint owner/repo
```

## Keyboard Controls

| Key | Action |
|-----|--------|
| Up/Down | Navigate rows |
| Left/Right | Switch between repositories |
| Enter | Open menu |
| q | Quit |

### Menu Options

- **Add Repository** - Track a new GitHub repository
- **Switch Project** - Open a different database file
- **Export Tab to Markdown** - Save current view as Markdown
- **Export Database Backup** - Copy database file
- **Highlight Domain** - Color-code email domains
- **Add Link/Tag** - Annotate committers
- **Quit** - Exit application

## Data Storage

Project databases (`.db` files) are created in your current working directory. Each project maintains:
- Tracked repositories
- Commit history
- Committer statistics
- Email domain highlights
- Links and tags

## Legacy Python Version

The original Python implementation is available in the `legacy_python/` directory.

## License

See LICENSE file for details.
