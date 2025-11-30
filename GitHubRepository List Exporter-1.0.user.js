// ==UserScript==
// @name         GitHub Repository List Exporter
// @namespace    http://tampermonkey.net/
// @version      1.0
// @description  Export all repositories from GitHub org/user pages in owner/repo format
// @author       You
// @match        https://github.com/orgs/*/repositories*
// @match        https://github.com/*?tab=repositories*
// @icon         https://github.githubassets.com/favicons/favicon.svg
// @grant        none
// ==/UserScript==

(function() {
    'use strict';

    // Extract owner/org name from URL
    function getOwnerFromURL() {
        const url = window.location.pathname;
        // Match /orgs/{owner}/repositories or /{owner}?tab=repositories
        const orgMatch = url.match(/^\/orgs\/([^\/]+)/);
        if (orgMatch) return orgMatch[1];

        const userMatch = url.match(/^\/([^\/]+)/);
        if (userMatch) return userMatch[1];

        return null;
    }

    // Scrape visible repositories from the page
    function scrapeVisibleRepos() {
        const repos = [];
        const repoLinks = document.querySelectorAll('a[itemprop="name codeRepository"]');

        repoLinks.forEach(link => {
            const fullPath = link.getAttribute('href');
            if (fullPath) {
                // Remove leading slash and extract owner/repo
                const cleaned = fullPath.replace(/^\//, '');
                repos.push(cleaned);
            }
        });

        return repos;
    }

    // Use GitHub API to fetch all repositories (more reliable)
    async function fetchAllRepos(owner) {
        const repos = [];
        let page = 1;
        const perPage = 100;

        // Determine if it's an org or user by checking current URL
        const isOrg = window.location.pathname.includes('/orgs/');
        const endpoint = isOrg ?
            `https://api.github.com/orgs/${owner}/repos` :
            `https://api.github.com/users/${owner}/repos`;

        try {
            while (true) {
                const response = await fetch(`${endpoint}?per_page=${perPage}&page=${page}`);

                if (!response.ok) {
                    throw new Error(`API request failed: ${response.status}`);
                }

                const data = await response.json();

                if (data.length === 0) break;

                data.forEach(repo => {
                    repos.push(`${repo.owner.login}/${repo.name}`);
                });

                if (data.length < perPage) break;
                page++;
            }
        } catch (error) {
            console.error('API fetch failed:', error);
            return null;
        }

        return repos;
    }

    // Export repositories to clipboard and download
    function exportRepos(repos, owner) {
        const content = repos.join('\n');
        const timestamp = new Date().toISOString().split('T')[0];
        const filename = `${owner}-repos-${timestamp}.txt`;

        // Copy to clipboard
        navigator.clipboard.writeText(content).then(() => {
            console.log('Copied to clipboard!');
        }).catch(err => {
            console.error('Failed to copy:', err);
        });

        // Download as file
        const blob = new Blob([content], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        a.click();
        URL.revokeObjectURL(url);

        return { count: repos.length, filename };
    }

    // Create and add export button
    function addExportButton() {
        const owner = getOwnerFromURL();
        if (!owner) return;
        
        // Create a container div
        const container = document.createElement('div');
        container.style.cssText = `
            display: inline-flex;
            align-items: center;
            margin-right: 8px;
        `;
        
        // Create button
        const button = document.createElement('button');
        button.textContent = 'Export Repos';
        button.type = 'button';
        button.style.cssText = `
            padding: 5px 16px;
            background: #238636;
            color: white;
            border: 1px solid rgba(240,246,252,0.1);
            border-radius: 6px;
            font-weight: 500;
            cursor: pointer;
            font-size: 14px;
            height: 32px;
            white-space: nowrap;
            transition: background 0.2s;
            line-height: 20px;
        `;
        
        button.addEventListener('mouseenter', () => {
            button.style.background = '#2ea043';
        });
        
        button.addEventListener('mouseleave', () => {
            button.style.background = '#238636';
        });
        
        button.addEventListener('click', async () => {
            // Disable button and show loading
            button.disabled = true;
            const originalText = button.textContent;
            button.textContent = 'Fetching...';
            button.style.background = '#656d76';
            button.style.cursor = 'wait';
            
            try {
                // Try API first (more reliable and gets all repos)
                let repos = await fetchAllRepos(owner);
                
                // Fallback to scraping if API fails
                if (!repos || repos.length === 0) {
                    repos = scrapeVisibleRepos();
                }
                
                if (repos.length === 0) {
                    button.textContent = 'No repos found';
                    button.style.background = '#da3633';
                    setTimeout(() => {
                        button.textContent = originalText;
                        button.style.background = '#238636';
                        button.style.cursor = 'pointer';
                        button.disabled = false;
                    }, 2000);
                    return;
                }
                
                const result = exportRepos(repos, owner);
                
                // Show success message
                button.textContent = `Exported ${result.count}`;
                button.style.background = '#2ea043';
                
                // Reset after 3 seconds
                setTimeout(() => {
                    button.textContent = originalText;
                    button.style.background = '#238636';
                    button.style.cursor = 'pointer';
                    button.disabled = false;
                }, 3000);
                
            } catch (error) {
                console.error('Export failed:', error);
                button.textContent = 'Export failed';
                button.style.background = '#da3633';
                
                setTimeout(() => {
                    button.textContent = originalText;
                    button.style.background = '#238636';
                    button.style.cursor = 'pointer';
                    button.disabled = false;
                }, 2000);
            }
        });
        
        container.appendChild(button);
        
        // Find the search container and insert before it
        const searchButton = document.querySelector('[data-target="qbsearch-input.inputButton"]');
        if (searchButton) {
            const searchContainer = searchButton.closest('.AppHeader-search');
            if (searchContainer && searchContainer.parentNode) {
                searchContainer.parentNode.insertBefore(container, searchContainer);
            } else {
                // Fallback to fixed position if we can't find proper container
                container.style.cssText = `
                    position: fixed;
                    top: 12px;
                    right: 320px;
                    z-index: 99;
                `;
                document.body.appendChild(container);
            }
        }
    }

    // Wait for page to load, then add button
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', addExportButton);
    } else {
        addExportButton();
    }
})();