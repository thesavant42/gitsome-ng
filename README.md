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
- Lists all repositories with ID, name, stars, forks, commits, branches, creation date, and URL
- Saves JSON to `output/{org_name}-org.json`
- Saves Markdown report to `output/{org_name}-org.md`

**Options:**
- `--print-repos`: Print detailed information for each repository (equivalent to running `repo` command for each repository)
- `--print-committers`: Print detailed information for each committer (equivalent to running `user` command for each). Works with `--print-repos` to show committers for each repository.
- `--stargazers`: Include stargazer information in output (only shown when this flag is used)

### Query Repository Details

Get detailed information about a specific repository:

```bash
python gitsome.py repo facebook graphql
```

**Output:**
- Repository metadata (description, URLs, settings)
- Pull requests (last 100)
- Commit statistics with committer information (from all commits)
- Branch information
- Saves JSON to `output/{owner}-{repo_name}-repo.json`
- Saves Markdown report to `output/{owner}-{repo_name}-repo.md`

**Options:**
- `--print-committers`: Print detailed information for each committer (equivalent to running `user` command for each committer)
- `--stargazers`: Include stargazer information in output (only shown when this flag is used)

### Query User Profile

Get information about a GitHub user:

```bash
python gitsome.py user octocat
```

**Output:**
- User profile information
- All repositories with details (compact format)
- Gists with file contents (for plain text files)
- Followers and following lists
- Saves JSON to `output/{username}-user.json`
- Saves Markdown report to `output/{username}-user.md`

**Options:**
- `--print-gists`: Print detailed gist information (note: basic gist info is always displayed)

### Global Options

- `--no-save`: Skip saving JSON and Markdown output files
- `-h, --help`: Show help message

### Command-Specific Options

**Organization (`org`) command:**
- `--print-repos`: Print detailed information for each repository
- `--print-committers`: Print detailed information for each committer (works with `--print-repos`)
- `--stargazers`: Include stargazer information in output

**Repository (`repo`) command:**
- `--print-committers`: Print detailed information for each committer
- `--stargazers`: Include stargazer information in output

**User (`user`) command:**
- `--print-gists`: Print detailed gist information (basic gist info is always displayed)

## Examples

```bash
# Query an organization
python gitsome.py org microsoft

# Query an organization and print detailed info for each repository
python gitsome.py org microsoft --print-repos

# Query an organization, print repos, and show committer details
python gitsome.py org microsoft --print-repos --print-committers

# Query a specific repository
python gitsome.py repo microsoft vscode

# Query a repository and show committer details
python gitsome.py repo microsoft vscode --print-committers

# Query a repository with stargazer information
python gitsome.py repo microsoft vscode --stargazers

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
