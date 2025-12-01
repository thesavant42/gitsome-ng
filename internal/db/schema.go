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

// Schema for user profiles (fetched from GitHub for tagged users)
const createUserProfilesTable = `
CREATE TABLE IF NOT EXISTS user_profiles (
    login TEXT PRIMARY KEY,
    name TEXT,
    bio TEXT,
    company TEXT,
    location TEXT,
    email TEXT,
    website_url TEXT,
    twitter_username TEXT,
    pronouns TEXT,
    avatar_url TEXT,
    follower_count INTEGER,
    following_count INTEGER,
    created_at TEXT,
    organizations TEXT,
    social_accounts TEXT,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// Schema for user repositories (fetched from GitHub for tagged users)
const createUserRepositoriesTable = `
CREATE TABLE IF NOT EXISTS user_repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    github_login TEXT NOT NULL,
    name TEXT,
    owner_login TEXT,
    description TEXT,
    url TEXT,
    ssh_url TEXT,
    homepage_url TEXT,
    disk_usage INTEGER,
    stargazer_count INTEGER,
    fork_count INTEGER,
    commit_count INTEGER,
    is_fork BOOLEAN,
    is_empty BOOLEAN,
    is_in_organization BOOLEAN,
    has_wiki_enabled BOOLEAN,
    visibility TEXT,
    primary_language TEXT,
    license_name TEXT,
    created_at TEXT,
    updated_at TEXT,
    pushed_at TEXT,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(github_login, owner_login, name)
);

CREATE INDEX IF NOT EXISTS idx_user_repos_login ON user_repositories(github_login);
`

// Schema for user gists
const createUserGistsTable = `
CREATE TABLE IF NOT EXISTS user_gists (
    id TEXT PRIMARY KEY,
    github_login TEXT NOT NULL,
    name TEXT,
    description TEXT,
    url TEXT,
    resource_path TEXT,
    is_public BOOLEAN,
    is_fork BOOLEAN,
    stargazer_count INTEGER,
    fork_count INTEGER,
    revision_count INTEGER,
    created_at TEXT,
    updated_at TEXT,
    pushed_at TEXT,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_gists_login ON user_gists(github_login);
`

// Schema for gist files
const createGistFilesTable = `
CREATE TABLE IF NOT EXISTS gist_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gist_id TEXT NOT NULL,
    name TEXT,
    encoded_name TEXT,
    extension TEXT,
    language TEXT,
    size INTEGER,
    encoding TEXT,
    is_image BOOLEAN,
    is_truncated BOOLEAN,
    text TEXT,
    FOREIGN KEY(gist_id) REFERENCES user_gists(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gist_files_gist ON gist_files(gist_id);
`

// Schema for gist comments
const createGistCommentsTable = `
CREATE TABLE IF NOT EXISTS gist_comments (
    id TEXT PRIMARY KEY,
    gist_id TEXT NOT NULL,
    author_login TEXT,
    body_text TEXT,
    created_at TEXT,
    updated_at TEXT,
    FOREIGN KEY(gist_id) REFERENCES user_gists(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gist_comments_gist ON gist_comments(gist_id);
`

// SQL queries for user profiles
const insertUserProfile = `
INSERT OR REPLACE INTO user_profiles (
    login, name, bio, company, location, email, website_url,
    twitter_username, pronouns, avatar_url, follower_count, following_count,
    created_at, organizations, social_accounts, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`

const selectUserProfile = `
SELECT login, name, bio, company, location, email, website_url,
       twitter_username, pronouns, avatar_url, follower_count, following_count,
       created_at, organizations, social_accounts, fetched_at
FROM user_profiles
WHERE login = ?
`

// SQL queries for user repositories
const insertUserRepository = `
INSERT OR REPLACE INTO user_repositories (
    github_login, name, owner_login, description, url, ssh_url, homepage_url,
    disk_usage, stargazer_count, fork_count, commit_count, is_fork, is_empty, is_in_organization,
    has_wiki_enabled, visibility, primary_language, license_name, created_at, updated_at, pushed_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`

const selectUserRepositories = `
SELECT id, github_login, name, owner_login, description, url, ssh_url, homepage_url,
       disk_usage, stargazer_count, fork_count, commit_count, is_fork, is_empty, is_in_organization,
       has_wiki_enabled, visibility, primary_language, license_name, created_at, updated_at, pushed_at, fetched_at
FROM user_repositories
WHERE github_login = ?
ORDER BY stargazer_count DESC, name ASC
`

const selectUserRepositoryCount = `
SELECT COUNT(*) FROM user_repositories WHERE github_login = ?
`

const deleteUserRepositories = `
DELETE FROM user_repositories WHERE github_login = ?
`

const deleteUserRepository = `
DELETE FROM user_repositories WHERE github_login = ? AND name = ?
`

const deleteCommitsByEmail = `
DELETE FROM commits WHERE repo_owner = ? AND repo_name = ? AND committer_email = ?
`

const updateCommitterLogin = `
UPDATE commits SET github_committer_login = ? WHERE repo_owner = ? AND repo_name = ? AND committer_email = ?
`

const updateCommitterName = `
UPDATE commits SET committer_name = ? WHERE repo_owner = ? AND repo_name = ? AND committer_email = ?
`

// SQL queries for user gists
const insertUserGist = `
INSERT OR REPLACE INTO user_gists (
    id, github_login, name, description, url, resource_path,
    is_public, is_fork, stargazer_count, fork_count, revision_count, created_at, updated_at, pushed_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`

const selectUserGists = `
SELECT id, github_login, name, description, url, resource_path,
       is_public, is_fork, stargazer_count, fork_count, revision_count, created_at, updated_at, pushed_at, fetched_at
FROM user_gists
WHERE github_login = ?
ORDER BY created_at DESC
`

const selectUserGistCount = `
SELECT COUNT(*) FROM user_gists WHERE github_login = ?
`

const deleteUserGists = `
DELETE FROM user_gists WHERE github_login = ?
`

const deleteUserGist = `
DELETE FROM user_gists WHERE github_login = ? AND id = ?
`

// SQL queries for gist files
const insertGistFile = `
INSERT OR REPLACE INTO gist_files (
    gist_id, name, encoded_name, extension, language, size, encoding, is_image, is_truncated, text
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const selectGistFiles = `
SELECT id, gist_id, name, encoded_name, extension, language, size, encoding, is_image, is_truncated, text
FROM gist_files
WHERE gist_id = ?
ORDER BY name ASC
`

const deleteGistFiles = `
DELETE FROM gist_files WHERE gist_id = ?
`

// SQL queries for gist comments
const insertGistComment = `
INSERT OR REPLACE INTO gist_comments (
    id, gist_id, author_login, body_text, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
`

const selectGistComments = `
SELECT id, gist_id, author_login, body_text, created_at, updated_at
FROM gist_comments
WHERE gist_id = ?
ORDER BY created_at ASC
`

const deleteGistComments = `
DELETE FROM gist_comments WHERE gist_id = ?
`

// Check if user has fetched data
const selectUserHasData = `
SELECT EXISTS(SELECT 1 FROM user_profiles WHERE login = ?)
   OR EXISTS(SELECT 1 FROM user_repositories WHERE github_login = ?)
   OR EXISTS(SELECT 1 FROM user_gists WHERE github_login = ?)
`

// Schema for API call logs
const createAPILogsTable = `
CREATE TABLE IF NOT EXISTS api_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    status_code INTEGER,
    error TEXT,
    rate_limit_remaining INTEGER,
    rate_limit_reset TEXT,
    login TEXT
);

CREATE INDEX IF NOT EXISTS idx_api_logs_timestamp ON api_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_api_logs_login ON api_logs(login);
`

// SQL queries for API logs
const insertAPILog = `
INSERT INTO api_logs (method, endpoint, status_code, error, rate_limit_remaining, rate_limit_reset, login)
VALUES (?, ?, ?, ?, ?, ?, ?)
`

const selectAPILogs = `
SELECT id, timestamp, method, endpoint, status_code, error, rate_limit_remaining, rate_limit_reset, login
FROM api_logs
ORDER BY timestamp DESC
LIMIT ?
`

const selectAPILogsByLogin = `
SELECT id, timestamp, method, endpoint, status_code, error, rate_limit_remaining, rate_limit_reset, login
FROM api_logs
WHERE login = ?
ORDER BY timestamp DESC
LIMIT ?
`

// Schema for layer inspections (Docker image layer peek history)
const createLayerInspectionsTable = `
CREATE TABLE IF NOT EXISTS layer_inspections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    image_ref TEXT NOT NULL,
    layer_digest TEXT NOT NULL,
    layer_index INTEGER NOT NULL,
    layer_size INTEGER,
    entry_count INTEGER,
    contents TEXT,
    downloaded BOOLEAN DEFAULT FALSE,
    download_path TEXT,
    inspected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(image_ref, layer_digest)
);

CREATE INDEX IF NOT EXISTS idx_layer_inspections_image ON layer_inspections(image_ref);
CREATE INDEX IF NOT EXISTS idx_layer_inspections_digest ON layer_inspections(layer_digest);
`

// SQL queries for layer inspections
const insertLayerInspection = `
INSERT OR REPLACE INTO layer_inspections (
    image_ref, layer_digest, layer_index, layer_size, entry_count, contents, downloaded, download_path, inspected_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`

const selectLayerInspection = `
SELECT id, image_ref, layer_digest, layer_index, layer_size, entry_count, contents, downloaded, download_path, inspected_at
FROM layer_inspections
WHERE image_ref = ? AND layer_digest = ?
`

const selectLayerInspectionByDigest = `
SELECT id, image_ref, layer_digest, layer_index, layer_size, entry_count, contents, downloaded, download_path, inspected_at
FROM layer_inspections
WHERE layer_digest = ?
ORDER BY inspected_at DESC
LIMIT 1
`

const selectLayerInspections = `
SELECT id, image_ref, layer_digest, layer_index, layer_size, entry_count, contents, downloaded, download_path, inspected_at
FROM layer_inspections
ORDER BY inspected_at DESC
LIMIT ?
`

const selectLayerInspectionsByImage = `
SELECT id, image_ref, layer_digest, layer_index, layer_size, entry_count, contents, downloaded, download_path, inspected_at
FROM layer_inspections
WHERE image_ref = ?
ORDER BY layer_index ASC
`

const updateLayerDownloaded = `
UPDATE layer_inspections SET downloaded = TRUE, download_path = ? WHERE image_ref = ? AND layer_digest = ?
`

// Schema for image manifests (stores build steps and other image metadata)
const createImageManifestsTable = `
CREATE TABLE IF NOT EXISTS image_manifests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    image_ref TEXT NOT NULL UNIQUE,
    platform TEXT,
    build_steps TEXT,
    config_digest TEXT,
    layer_count INTEGER,
    total_size INTEGER,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_image_manifests_ref ON image_manifests(image_ref);
`

// SQL queries for image manifests
const insertImageManifest = `
INSERT OR REPLACE INTO image_manifests (
    image_ref, platform, build_steps, config_digest, layer_count, total_size, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`

const selectImageManifest = `
SELECT id, image_ref, platform, build_steps, config_digest, layer_count, total_size, fetched_at
FROM image_manifests
WHERE image_ref = ?
`

const selectImageManifestBuildSteps = `
SELECT build_steps FROM image_manifests WHERE image_ref = ?
`
