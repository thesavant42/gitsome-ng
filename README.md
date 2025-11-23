# gitsome

GitHub Info Enumerator - Unified Python script for querying GitHub via GraphQL API

A unified Python port of the original bash scripts for enumerating GitHub organizations, repositories, and users.

## Features

- Query organization repositories and metadata
- Query detailed repository information (PRs, issues, stargazers, releases, etc.)
- Query user profiles, repositories, gists, and comments
- Save JSON and Markdown reports for further analysis
- Formatted console output
- Error handling for authentication and rate limits
- Modular codebase structure for easy maintenance and extension

## Installation

1. Install Python 3.7 or higher
2. Install dependencies:
```bash
pip install -r requirements.txt
```

## Authentication

gitsome requires a GitHub Personal Access Token (PAT) to authenticate with the GitHub GraphQL API.

### Setting up your GitHub Token

**Set the environment variable:**
   
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
   
   **To make it persistent (Linux/macOS):**
   Add to your `~/.bashrc` or `~/.zshrc`:
   ```bash
   export GITHUB_TOKEN="your_token_here"
   ```

## Usage

### Query Organization Repositories

Get information about all repositories in an organization:

```bash
python gitsome.py org thesavant42
```

**Output:**
- Lists all repositories with ID, name, stars, forks, creation date, and URL
- Saves JSON to `output/{org_name}-org.json`
- Saves Markdown report to `output/{org_name}-org.md`

### Query Repository Details

Get detailed information about a specific repository:

```bash
python gitsome.py repo facebook graphql
```

**Output:**
- Repository metadata (description, URLs, settings)
- Pull requests (last 100)
- Issues (last 25)
- Assignable users with their gists and repositories
- Stargazers (last 10)
- Releases (last 100)
- Branches and commit history
- Saves JSON to `output/{owner}-{repo_name}-repo.json`
- Saves Markdown report to `output/{owner}-{repo_name}-repo.md`

### Query User Profile

Get information about a GitHub user:

```bash
python gitsome.py user octocat
```

**Output:**
- User profile information
- All repositories with details
- Gists and gist comments
- Followers and following counts
- Clone URLs for all repositories
- Saves JSON to `output/{username}-user.json`
- Saves Markdown report to `output/{username}-user.md`

### Options

- `--no-save`: Skip saving JSON and Markdown output files
- `-h, --help`: Show help message

## Examples

```bash
# Query an organization
python gitsome.py org microsoft

# Query a specific repository
python gitsome.py repo microsoft vscode

# Query a user profile
python gitsome.py user github

# Query without saving JSON
python gitsome.py org thesavant42 --no-save
```

## Default Values

- Organization: `thesavant42` (if not specified)
- Repository owner: `thesavant42` (if not specified)
- Repository name: `gitsome` (if not specified)

## Error Handling

The script handles common errors:
- **401 Unauthorized**: Invalid or missing token
- **403 Forbidden**: Rate limit exceeded or insufficient permissions
- **GraphQL Errors**: Displays specific error messages from GitHub API

## Original Scripts

This Python script is a unified port of the original bash scripts:
- `gitsome-by-org.sh` - Organization enumeration
- `gitsome-by-repo.sh` - Repository enumeration  
- `gitsome-by-user.sh` - User enumeration

## License

See LICENSE file for details.

## Author

by savant42
