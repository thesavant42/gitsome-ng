package db

const createCommitsTable = `
CREATE TABLE IF NOT EXISTS commits (
    sha TEXT PRIMARY KEY,
    message TEXT,
    author_name TEXT,
    author_email TEXT,
    author_date TEXT,
    committer_name TEXT,
    committer_email TEXT,
    committer_date TEXT,
    github_author_login TEXT,
    github_committer_login TEXT,
    html_url TEXT,
    repo_owner TEXT,
    repo_name TEXT
);

CREATE INDEX IF NOT EXISTS idx_commits_repo ON commits(repo_owner, repo_name);
CREATE INDEX IF NOT EXISTS idx_commits_committer ON commits(committer_name, committer_email);
CREATE INDEX IF NOT EXISTS idx_commits_author ON commits(author_name, author_email);
`

const insertCommit = `
INSERT OR REPLACE INTO commits (
    sha, message, author_name, author_email, author_date,
    committer_name, committer_email, committer_date,
    github_author_login, github_committer_login,
    html_url, repo_owner, repo_name
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const selectCommitterStats = `
SELECT 
    committer_name,
    committer_email,
    COALESCE(github_committer_login, '') as github_login,
    COUNT(*) as commit_count
FROM commits
WHERE repo_owner = ? AND repo_name = ?
GROUP BY committer_name, committer_email
ORDER BY commit_count DESC
`

const selectAuthorStats = `
SELECT 
    author_name,
    author_email,
    COALESCE(github_author_login, '') as github_login,
    COUNT(*) as commit_count
FROM commits
WHERE repo_owner = ? AND repo_name = ?
GROUP BY author_name, author_email
ORDER BY commit_count DESC
`

const selectTotalCommits = `
SELECT COUNT(*) FROM commits WHERE repo_owner = ? AND repo_name = ?
`

// Schema for committer links (grouping same person's different accounts)
const createLinksTable = `
CREATE TABLE IF NOT EXISTS committer_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    committer_email TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repo_owner, repo_name, committer_email)
);

CREATE INDEX IF NOT EXISTS idx_links_repo ON committer_links(repo_owner, repo_name);
CREATE INDEX IF NOT EXISTS idx_links_group ON committer_links(group_id);
`

// Schema for tagged committers
const createTagsTable = `
CREATE TABLE IF NOT EXISTS committer_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    committer_email TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repo_owner, repo_name, committer_email)
);

CREATE INDEX IF NOT EXISTS idx_tags_repo ON committer_tags(repo_owner, repo_name);
`

// SQL queries for links
const insertLink = `
INSERT OR REPLACE INTO committer_links (group_id, repo_owner, repo_name, committer_email)
VALUES (?, ?, ?, ?)
`

const selectLinks = `
SELECT committer_email, group_id FROM committer_links
WHERE repo_owner = ? AND repo_name = ?
`

const selectMaxGroupID = `
SELECT COALESCE(MAX(group_id), 0) FROM committer_links
WHERE repo_owner = ? AND repo_name = ?
`

const deleteLink = `
DELETE FROM committer_links WHERE repo_owner = ? AND repo_name = ? AND committer_email = ?
`

// SQL queries for tags
const insertTag = `
INSERT OR IGNORE INTO committer_tags (repo_owner, repo_name, committer_email)
VALUES (?, ?, ?)
`

const selectTags = `
SELECT committer_email FROM committer_tags
WHERE repo_owner = ? AND repo_name = ?
`

const deleteTag = `
DELETE FROM committer_tags WHERE repo_owner = ? AND repo_name = ? AND committer_email = ?
`

// Schema for highlight domains (email domains to highlight)
const createDomainsTable = `
CREATE TABLE IF NOT EXISTS highlight_domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    domain TEXT NOT NULL,
    color_index INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repo_owner, repo_name, domain)
);

CREATE INDEX IF NOT EXISTS idx_domains_repo ON highlight_domains(repo_owner, repo_name);
`

// SQL queries for highlight domains (global - shared across all repos)
const insertDomain = `
INSERT OR REPLACE INTO highlight_domains (repo_owner, repo_name, domain, color_index)
VALUES ('_global_', '_global_', ?, ?)
`

const selectDomains = `
SELECT domain, color_index FROM highlight_domains
ORDER BY created_at ASC
`

const selectMaxDomainColorIndex = `
SELECT COALESCE(MAX(color_index), -1) FROM highlight_domains
`

const deleteDomain = `
DELETE FROM highlight_domains WHERE domain = ?
`

// Schema for tracked repositories
const createTrackedReposTable = `
CREATE TABLE IF NOT EXISTS tracked_repos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repo_owner, repo_name)
);
`

// SQL queries for tracked repos
const insertTrackedRepo = `
INSERT OR IGNORE INTO tracked_repos (repo_owner, repo_name)
VALUES (?, ?)
`

const selectTrackedRepos = `
SELECT repo_owner, repo_name, added_at FROM tracked_repos
ORDER BY added_at ASC
`

const deleteTrackedRepo = `
DELETE FROM tracked_repos WHERE repo_owner = ? AND repo_name = ?
`

// Combined stats across all repos - deduplicates by GitHub login (falls back to email)
const selectCombinedCommitterStats = `
SELECT 
    committer_name,
    committer_email,
    COALESCE(github_committer_login, '') as github_login,
    SUM(commit_count) as total_commits
FROM (
    SELECT 
        committer_name,
        committer_email,
        github_committer_login,
        COUNT(*) as commit_count,
        CASE 
            WHEN github_committer_login IS NOT NULL AND github_committer_login != '' 
            THEN github_committer_login 
            ELSE committer_email 
        END as dedup_key
    FROM commits
    GROUP BY committer_name, committer_email, github_committer_login
)
GROUP BY dedup_key
ORDER BY total_commits DESC
`

const selectCombinedTotalCommits = `
SELECT COUNT(*) FROM commits
`

