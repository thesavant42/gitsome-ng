"""Repository handler - query, format, and report generation for repositories"""

from __future__ import annotations

import sys
from typing import Dict, Any, Optional

# Import for runtime use
from gitsome.client import GitHubGraphQLClient


def get_repo_query(owner: str, repo_name: str, default_branch: str = None, include_stargazers: bool = False) -> str:
    """Generate GraphQL query for repository details"""
    stargazers_section = ""
    if include_stargazers:
        stargazers_section = """
        stargazerCount
        stargazers(last: 10) {{
          totalCount
          nodes {{
            name
            login
            email
            company
            url
          }}
        }}"""
    
    return f"""
    query {{
      repository(owner: "{owner}", name: "{repo_name}") {{
        id
        nameWithOwner
        description
        url
        homepageUrl
        mirrorUrl
        projectsUrl
        projectsV2(first: 100) {{
          totalCount
        }}
        contactLinks {{
          name
          url
          about
        }}
        diskUsage
        hasWikiEnabled
        codeOfConduct {{
          name
          url
        }}
        hasVulnerabilityAlertsEnabled
        isArchived
        isBlankIssuesEnabled
        isDisabled
        isEmpty
        isInOrganization
        isLocked
        isMirror
        isPrivate
        isSecurityPolicyEnabled
        isTemplate
        isUserConfigurationRepository
        isFork
        hasProjectsEnabled
        hasIssuesEnabled
        pullRequests(last: 100) {{
          totalCount
          nodes {{
            number
            author {{
              login
              url
              resourcePath
            }}
            bodyText
            permalink
          }}
        }}
        assignableUsers(last: 100) {{
          totalCount
          nodes {{
            name
            login
            company
            location
            pronouns
            status {{
              message
              emoji
            }}
            topRepositories(last: 100, orderBy: {{field: UPDATED_AT, direction: DESC}}) {{
              totalCount
              edges {{
                node {{
                  name
                  description
                  url
                }}
              }}
            }}
            repositories(first: 100, orderBy: {{ field: UPDATED_AT, direction: DESC}}) {{
              totalCount
              edges {{
                node {{
                  name
                }}
              }}
            }}
            gists(last: 100, orderBy: {{ field: CREATED_AT, direction: DESC }}) {{
              totalCount
              edges {{
                node {{
                  name
                  description
                  url
                  files {{
                    encodedName
                    language {{
                      name
                    }}
                    size
                  }}
                  updatedAt
                }}
              }}
            }}
          }}
        }}
        issues(last: 25) {{
          totalCount
          edges {{
            node {{
              number
              title
              author {{
                login
              }}
              bodyText
              bodyUrl
              createdAt
              lastEditedAt
              closed
            }}
          }}
        }}
        forkCount
        packages(last: 100) {{
          totalCount
        }}
        releases(last: 100) {{
          totalCount
          nodes {{
            name
            url
            author {{
              name
              login
              email
            }}
            createdAt
            description
            isDraft
            isLatest
            isPrerelease
            databaseId
            resourcePath
          }}
        }}{stargazers_section}
        defaultBranchRef {{
          name
          target {{
            ... on Commit {{
              history {{
                totalCount
              }}
            }}
          }}
        }}
        object(expression: "HEAD") {{
          ... on Commit {{
            history(first: 100) {{
              totalCount
              pageInfo {{
                endCursor
                hasNextPage
              }}
              nodes {{
                author {{
                  name
                  email
                  user {{
                    login
                  }}
                }}
              }}
            }}
          }}
        }}
        refs(refPrefix: "refs/heads/", first: 100) {{
          totalCount
          nodes {{
            name
            target {{
              ... on Commit {{
                oid
              }}
            }}
          }}
        }}
      }}
    }}
    """


def build_branch_comparison_query(owner: str, repo_name: str, base_branch: str, branch_names: list) -> str:
    """Build a GraphQL query with aliases to compare multiple branches at once"""
    aliases = []
    for i, branch_name in enumerate(branch_names[:50]):  # Limit to 50 to avoid query size issues
        # Create a safe alias name (GraphQL aliases can't have special chars)
        alias = f"ref_{i}".replace('-', '_').replace('.', '_')
        aliases.append(f'{alias}: ref(qualifiedName: "refs/heads/{branch_name}") {{ compare(headRef: "refs/heads/{base_branch}") {{ aheadBy behindBy }} }}')
    
    aliases_str = '\n        '.join(aliases)
    
    return f"""
    query {{
      repository(owner: "{owner}", name: "{repo_name}") {{
        {aliases_str}
      }}
    }}
    """


def generate_markdown_report(data: Dict[str, Any], owner: str, repo_name: str, client: Optional["GitHubGraphQLClient"] = None, print_stargazers: bool = False) -> str:
    """Generate markdown report for repository"""
    repo = data.get('data', {}).get('repository')
    if not repo:
        return ""
    
    md_lines = []
    md_lines.append(f"# Repository: {repo_name}")
    md_lines.append(f"**Owner:** [{owner}](https://github.com/{owner})")
    md_lines.append("")
    
    # Repository Information Table
    md_lines.append("## Repository Information")
    md_lines.append("")
    md_lines.append("| Field | Value |")
    md_lines.append("|-------|-------|")
    md_lines.append(f"| Name (with Owner) | {repo.get('nameWithOwner', 'N/A')} |")
    md_lines.append(f"| ID | `{repo.get('id', 'N/A')}` |")
    md_lines.append(f"| Description | {repo.get('description', 'N/A') or 'N/A'} |")
    md_lines.append(f"| URL | [{repo.get('url', 'N/A')}]({repo.get('url', '#')}) |")
    md_lines.append(f"| Homepage | {repo.get('homepageUrl', 'N/A') or 'N/A'} |")
    md_lines.append(f"| Mirror URL | {repo.get('mirrorUrl', 'N/A') or 'N/A'} |")
    md_lines.append(f"| Projects Count | {repo.get('projectsV2', {}).get('totalCount', 0)} |")
    md_lines.append(f"| Disk Usage | {repo.get('diskUsage', 'N/A')} |")
    md_lines.append(f"| Has Wiki | {repo.get('hasWikiEnabled', False)} |")
    md_lines.append(f"| Is in Org | {repo.get('isInOrganization', False)} |")
    md_lines.append(f"| Is Empty | {repo.get('isEmpty', False)} |")
    md_lines.append(f"| Is Mirror | {repo.get('isMirror', False)} |")
    md_lines.append(f"| Is Fork | {repo.get('isFork', False)} |")
    md_lines.append(f"| Has Projects Enabled | {repo.get('hasProjectsEnabled', False)} |")
    md_lines.append(f"| Has Issues Enabled | {repo.get('hasIssuesEnabled', False)} |")
    if print_stargazers:
        md_lines.append(f"| Total Stargazers | {repo.get('stargazerCount', 0)} |")
    
    # Add zip download link
    default_branch = repo.get('defaultBranchRef', {})
    default_branch_name = default_branch.get('name', 'main')
    zip_url = f"https://github.com/{owner}/{repo_name}/archive/refs/heads/{default_branch_name}.zip"
    md_lines.append(f"| Download ZIP | [{zip_url}]({zip_url}) |")
    md_lines.append("")
    
    # Commit Statistics
    default_branch = repo.get('defaultBranchRef', {})
    default_branch_name = default_branch.get('name', 'main')
    commit_history = default_branch.get('target', {}).get('history', {})
    total_commits = commit_history.get('totalCount', 0)
    
    # Get committers from history
    commit_object = repo.get('object')
    commit_nodes = []
    if commit_object:
        commit_history = commit_object.get('history', {})
        if commit_history:
            commit_nodes = commit_history.get('nodes', [])
    
    committer_stats = {}
    
    for commit in commit_nodes:
        author = commit.get('author', {})
        user_login = author.get('user', {}).get('login') if author.get('user') else None
        author_name = author.get('name', 'Unknown')
        author_email = author.get('email', '')
        
        # Use login if available, otherwise use name+email
        key = user_login if user_login else f"{author_name} <{author_email}>"
        
        # Store committer info: (count, email, login)
        if key not in committer_stats:
            committer_stats[key] = {'count': 0, 'email': author_email, 'login': user_login}
        committer_stats[key]['count'] += 1
    
    # Sort committers by commit count (descending)
    sorted_committers = sorted(committer_stats.items(), key=lambda x: x[1]['count'], reverse=True)
    total_committers = len(sorted_committers)
    
    md_lines.append("## Commit Statistics")
    md_lines.append("")
    md_lines.append(f"| Total Commits | Total Committers (from last 100 commits) |")
    md_lines.append(f"|---------------|------------------------------------------|")
    md_lines.append(f"| {total_commits} | {total_committers} |")
    md_lines.append("")
    
    if sorted_committers:
        md_lines.append("### Committers (ranked by commit count)")
        md_lines.append("")
        md_lines.append("| Rank | Committer | Commits |")
        md_lines.append("|------|-----------|---------|")
        for rank, (committer, info) in enumerate(sorted_committers[:10], 1):  # Top 10
            email_str = f" ({info['email']})" if info['email'] else ""
            # If we have a login, create a link; otherwise just show the name+email
            if info['login']:
                committer_display = f"[{info['login']}](https://github.com/{info['login']}){email_str}"
            else:
                committer_display = f"{committer}{email_str}"
            md_lines.append(f"| {rank} | {committer_display} | {info['count']} |")
        md_lines.append("")
    
    # Branch Information
    refs = repo.get('refs', {})
    total_branches = refs.get('totalCount', 0)
    branch_nodes = refs.get('nodes', [])
    
    md_lines.append("## Branch Information")
    md_lines.append("")
    branch_overview_url = f"https://github.com/{owner}/{repo_name}/branches/all"
    md_lines.append(f"**Total Branches:** [{total_branches}]({branch_overview_url})")
    md_lines.append("")
    
    if branch_nodes and repo.get('isFork'):
        ahead_branches = []
        
        for branch in branch_nodes:
            branch_name = branch.get('name', 'N/A')
            if branch_name == default_branch_name:
                continue
            
            # Get comparison data from the refs query (already included)
            comparison = branch.get('compare', {})
            if not comparison:
                continue
            
            ahead_by = comparison.get('aheadBy', 0)
            behind_by = comparison.get('behindBy', 0)
            
            # Only include branches that are ahead
            if ahead_by > 0:
                ahead_branches.append((branch_name, ahead_by, behind_by))
        
        if ahead_branches:
            # Sort by ahead_by descending
            ahead_branches.sort(key=lambda x: x[1], reverse=True)
            md_lines.append(f"### Branch Analysis - Branches ahead of \"{default_branch_name}\"")
            md_lines.append("")
            md_lines.append("| Rank | Branch | Commits Ahead | Commits Behind |")
            md_lines.append("|------|--------|---------------|----------------|")
            for rank, (branch_name, ahead, behind) in enumerate(ahead_branches, 1):
                # Create links: tree view and compare view
                tree_url = f"https://github.com/{owner}/{repo_name}/tree/{branch_name}"
                compare_url = f"https://github.com/{owner}/{repo_name}/compare/{default_branch_name}...{branch_name}"
                branch_link = f"[{branch_name}]({tree_url}) ([compare]({compare_url}))"
                behind_str = f"{behind}" if behind > 0 else "-"
                md_lines.append(f"| {rank} | {branch_link} | {ahead} | {behind_str} |")
            md_lines.append("")
    
    return "\n".join(md_lines)


def print_repo_info(data: Dict[str, Any], owner: str, repo_name: str, client: Optional["GitHubGraphQLClient"] = None, print_committers: bool = False, print_stargazers: bool = False) -> None:
    """Print formatted repository information"""
    print(f"[+] Repository: {repo_name}")
    print(f"  Owner: {owner}\n")
    
    repo = data.get('data', {}).get('repository')
    if not repo:
        # Check if there are errors in the response
        errors = data.get('errors', [])
        if errors:
            print(f"Error: {errors[0].get('message', 'Repository not found or not accessible')}", file=sys.stderr)
        else:
            print("Error: Repository not found or not accessible", file=sys.stderr)
        return
    
    print(f"  Name (with Owner): {repo.get('nameWithOwner')} | ID: {repo.get('id')}")
    print(f"  Description: {repo.get('description', 'N/A')}")
    print(f"  URL: {repo.get('url')}")
    homepage = repo.get('homepageUrl', '')
    mirror_url = repo.get('mirrorUrl')
    print(f"  Homepage: {homepage if homepage else 'None'} | Mirror URL: {mirror_url if mirror_url else 'None'}")
    print(f"  Projects Count: {repo.get('projectsV2', {}).get('totalCount', 0)} | Disk Usage: {repo.get('diskUsage', 'N/A')} | Has Wiki: {repo.get('hasWikiEnabled')} | Is in Org: {repo.get('isInOrganization')}")
    print(f"  Is Empty: {repo.get('isEmpty')} | Is Mirror: {repo.get('isMirror')} | Is Fork: {repo.get('isFork')}")
    print(f"  Has Projects Enabled: {repo.get('hasProjectsEnabled')} | Has Issues Enabled: {repo.get('hasIssuesEnabled')}")
    
    if print_stargazers:
        print(f"\n  Total Stargazers: {repo.get('stargazerCount', 0)}\n")
        stargazers = repo.get('stargazers', {}).get('nodes', [])
        if stargazers:
            print("  Stargazers:")
            for stargazer in stargazers:
                name = stargazer.get('name', 'None')
                email = stargazer.get('email', '')
                print(f"    Login: {stargazer.get('login')} Name: {name}")
                print(f"    Email: {email if email else ''} URL: {stargazer.get('url')}")
                print()
    
    prs = repo.get('pullRequests', {})
    print(f"\n  [*] Pull Requests: {prs.get('totalCount', 0)}\n")
    for pr in prs.get('nodes', []):
        print(f"  [+] PR #{pr.get('number')}")
        print(f"    Permalink: {pr.get('permalink')}")
        print(f"    Author: {pr.get('author', {}).get('login', 'N/A')}")
        print(f"    Body: {pr.get('bodyText', 'N/A')[:200]}...")
        print("    ---")
        print()
    
    # Commit Statistics
    default_branch = repo.get('defaultBranchRef', {})
    default_branch_name = default_branch.get('name', 'main')
    commit_history = default_branch.get('target', {}).get('history', {})
    total_commits = commit_history.get('totalCount', 0)
    
    print(f"\n  [+] Commit Statistics")
    print(f"  Total Commits: {total_commits}")
    
    # Get committers from history - paginate through all commits
    commit_object = repo.get('object')
    commit_nodes = []
    if commit_object:
        commit_history = commit_object.get('history', {})
        if commit_history:
            commit_nodes = commit_history.get('nodes', [])
            page_info = commit_history.get('pageInfo', {})
            
            # Paginate through all commits if there are more pages
            if client and page_info.get('hasNextPage'):
                cursor = page_info.get('endCursor')
                while cursor:
                    # Fetch next page of commits
                    pagination_query = f"""
                    query {{
                      repository(owner: "{owner}", name: "{repo_name}") {{
                        object(expression: "HEAD") {{
                          ... on Commit {{
                            history(first: 100, after: "{cursor}") {{
                              pageInfo {{
                                endCursor
                                hasNextPage
                              }}
                              nodes {{
                                author {{
                                  name
                                  email
                                  user {{
                                    login
                                  }}
                                }}
                              }}
                            }}
                          }}
                        }}
                      }}
                    }}
                    """
                    try:
                        pagination_data = client.query(pagination_query)
                        pagination_repo = pagination_data.get('data', {}).get('repository', {})
                        pagination_object = pagination_repo.get('object', {})
                        if pagination_object:
                            pagination_history = pagination_object.get('history', {})
                            if pagination_history:
                                commit_nodes.extend(pagination_history.get('nodes', []))
                                page_info = pagination_history.get('pageInfo', {})
                                cursor = page_info.get('endCursor') if page_info.get('hasNextPage') else None
                            else:
                                cursor = None
                        else:
                            cursor = None
                    except Exception as e:
                        print(f"  [Warning] Error fetching additional commits: {str(e)}", file=sys.stderr)
                        cursor = None
    
    committer_stats = {}
    
    for commit in commit_nodes:
        author = commit.get('author', {})
        user_login = author.get('user', {}).get('login') if author.get('user') else None
        author_name = author.get('name', 'Unknown')
        author_email = author.get('email', '')
        
        # Use login if available, otherwise use name+email
        key = user_login if user_login else f"{author_name} <{author_email}>"
        
        # Store committer info: (count, email, login)
        if key not in committer_stats:
            committer_stats[key] = {'count': 0, 'email': author_email, 'login': user_login}
        committer_stats[key]['count'] += 1
    
    # Sort committers by commit count (descending)
    sorted_committers = sorted(committer_stats.items(), key=lambda x: x[1]['count'], reverse=True)
    
    print(f"  Total Committers (from total commits): {len(sorted_committers)}")
    if sorted_committers:
        print("\n  Committers (ranked by commit count):")
        for rank, (committer, info) in enumerate(sorted_committers, 1):
            email_str = f" <{info['email']}>" if info.get('email') else ""
            print(f"    {rank}. {committer}{email_str}: {info.get('count', 0)} commits")
    
    # If --print-committers flag is set, loop through each committer and print user details
    if print_committers and client and sorted_committers:
        print("\nDetailed Committer Information")
        print("="*80 + "\n")
        
        # Import here to avoid circular imports
        from gitsome.handlers.user_handler import get_user_query, print_user_info
        
        for rank, (committer, info) in enumerate(sorted_committers, 1):
            user_login = info.get('login')
            
            if user_login:
                # This committer has a GitHub login, query user details
                print(f"Committer #{rank}: {user_login}")
                print("="*80 + "\n")
                
                try:
                    user_query = get_user_query(user_login)
                    user_data = client.query(user_query)
                    print_user_info(user_data, user_login)
                except Exception as e:
                    print(f"Error querying user {user_login}: {str(e)}", file=sys.stderr)
                    print()
            else:
                # This is a "name <email>" format, no login available
                print(f"Committer #{rank}: {committer}")
                print("="*80)
                print(f"Note: No GitHub user account found for this committer (email-based commit)")
                print()
        
        print()
    
    # Branch Information
    refs = repo.get('refs', {})
    total_branches = refs.get('totalCount', 0)
    branch_nodes = refs.get('nodes', [])
    
    print(f"\n  [+] Branch Information")
    print(f"  Total Branches: {total_branches}")
    
    if branch_nodes and repo.get('isFork'):
        ahead_branches = []
        
        for branch in branch_nodes:
            branch_name = branch.get('name', 'N/A')
            if branch_name == default_branch_name:
                continue
            
            # Get comparison data from the refs query (already included)
            comparison = branch.get('compare', {})
            if not comparison:
                continue
            
            ahead_by = comparison.get('aheadBy', 0)
            behind_by = comparison.get('behindBy', 0)
            
            # Only include branches that are ahead
            if ahead_by > 0:
                ahead_branches.append((branch_name, ahead_by, behind_by))
        
        if ahead_branches:
            # Sort by ahead_by descending
            ahead_branches.sort(key=lambda x: x[1], reverse=True)
            print(f"\n  [*] Fork Analysis - Branches ahead of \"{default_branch_name}\":")
            print(f"  Branches AHEAD of {default_branch_name} (ranked by commits ahead):")
            for rank, (branch_name, ahead, behind) in enumerate(ahead_branches, 1):
                behind_str = f" ({behind} commits behind)" if behind > 0 else ""
                print(f"    {rank}. {branch_name}: {ahead} commits ahead{behind_str}")

