"""Organization handler - query, format, and report generation for organizations"""

from typing import Dict, Any


def get_org_query(org_name: str) -> str:
    """Generate GraphQL query for organization repositories"""
    return f"""
    query {{
      repositoryOwner(login: "{org_name}") {{
        ... on Organization {{
          repositories(first: 100) {{
            totalCount
            pageInfo {{
              endCursor
              hasNextPage
            }}
            nodes {{
              id
              name
              stars: stargazerCount
              forks: forkCount
              created_at: createdAt
              organization: owner {{
                login
              }}
              repo_url: url
              defaultBranchRef {{
                target {{
                  ... on Commit {{
                    history {{
                      totalCount
                    }}
                  }}
                }}
              }}
              refs(refPrefix: "refs/heads/", first: 1) {{
                totalCount
              }}
            }}
          }}
        }}
      }}
    }}
    """


def generate_markdown_report_org(data: Dict[str, Any], org_name: str) -> str:
    """Generate markdown report for organization"""
    owner = data.get('data', {}).get('repositoryOwner')
    if not owner:
        return ""
    
    org_data = owner.get('repositories', {})
    total_count = org_data.get('totalCount', 0)
    nodes = org_data.get('nodes', [])
    
    md_lines = []
    md_lines.append(f"# Organization: {org_name}")
    md_lines.append(f"**GitHub Profile:** [{org_name}](https://github.com/{org_name})")
    md_lines.append("")
    
    md_lines.append(f"**Total Repositories:** {total_count}")
    md_lines.append("")
    
    if nodes:
        md_lines.append("## Repositories")
        md_lines.append("")
        md_lines.append("| Repository Name | Stars | Forks | Created | URL |")
        md_lines.append("|----------------|-------|-------|---------|-----|")
        for repo in nodes:
            repo_name = repo.get('name', 'N/A')
            org_login = repo.get('organization', {}).get('login', org_name)
            repo_url = repo.get('repo_url', '#')
            repo_link = f"[{repo_name}]({repo_url})"
            md_lines.append(f"| {repo_link} | {repo.get('stars', 0)} | {repo.get('forks', 0)} | {repo.get('created_at', 'N/A')} | [{repo_url}]({repo_url}) |")
        md_lines.append("")
    
    return "\n".join(md_lines)


def print_org_info(data: Dict[str, Any], org_name: str) -> None:
    """Print formatted organization information"""
    print(f"[+] Org: {org_name}\n")
    
    owner = data.get('data', {}).get('repositoryOwner')
    if not owner:
        print("Error: Organization not found or not accessible")
        return
    
    org_data = owner.get('repositories', {})
    total_count = org_data.get('totalCount', 0)
    nodes = org_data.get('nodes', [])
    
    print(f"Total Repositories: {total_count}\n")
    for repo in nodes:
        repo_name = repo.get('name')
        repo_id = repo.get('id')
        stars = repo.get('stars', 0)
        forks = repo.get('forks', 0)
        
        # Get commit count
        default_branch = repo.get('defaultBranchRef', {})
        commit_count = 0
        if default_branch:
            target = default_branch.get('target', {})
            if target:
                history = target.get('history', {})
                commit_count = history.get('totalCount', 0)
        
        # Get branch count
        refs = repo.get('refs', {})
        branch_count = refs.get('totalCount', 0)
        
        created = repo.get('created_at', 'N/A')
        repo_url = repo.get('repo_url', 'N/A')
        
        print(f"Repository: {repo_name} ID: {repo_id}")
        print(f"  Stars: {stars} Forks: {forks} Commits: {commit_count} Branches: {branch_count}")
        print(f"  Created: {created} URL: {repo_url}")
        print()

