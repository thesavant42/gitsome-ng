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
        print(f"Repository: {repo.get('name')}")
        print(f"  ID: {repo.get('id')}")
        print(f"  Stars: {repo.get('stars', 0)}")
        print(f"  Forks: {repo.get('forks', 0)}")
        print(f"  Created: {repo.get('created_at', 'N/A')}")
        print(f"  URL: {repo.get('repo_url', 'N/A')}")
        print()

