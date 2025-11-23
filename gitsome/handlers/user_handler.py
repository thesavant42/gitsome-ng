"""User handler - query, format, and report generation for users"""

from datetime import datetime
from typing import Dict, Any


def get_user_query(username: str) -> str:
    """Generate GraphQL query for user details"""
    return f"""
    query GetUserDetails {{
      user(login: "{username}") {{
        login
        company
        email
        location
        followers {{
          totalCount
        }}
        following(first: 100) {{
          totalCount
          nodes {{
            login
            name
            email
            url
            company
            location
          }}
        }}
        gists(first: 100, orderBy: {{ field: CREATED_AT, direction: DESC }}) {{
          totalCount
          edges {{
            node {{
              id
              name
              description
              url
              resourcePath
              isPublic
              isFork
              pushedAt
              createdAt
              updatedAt
              stargazerCount
              viewerHasStarred
              owner {{
                login
                id
              }}
              files {{
                name
                encodedName
                encoding
                extension
                isImage
                isTruncated
                language {{
                  name
                }}
                size
                text
              }}
              comments(first: 100) {{
                totalCount
                nodes {{
                  id
                  author {{
                    login
                    url
                  }}
                  bodyText
                  createdAt
                  updatedAt
                }}
              }}
              stargazers(first: 100) {{
                totalCount
                nodes {{
                  login
                  name
                  email
                  url
                }}
              }}
              forks(first: 100) {{
                totalCount
                nodes {{
                  name
                  url
                  owner {{
                    login
                  }}
                }}
              }}
            }}
          }}
        }}
        gistComments(first: 100) {{
          totalCount
          nodes {{
            author {{
              login
              url
            }}
            id
            body
            createdAt
            updatedAt
            gist {{
              id
              url
            }}
          }}
        }}
        repositories(first: 100, orderBy: {{ field: CREATED_AT, direction: DESC }}) {{
          totalCount
          pageInfo {{
            endCursor
            hasNextPage
          }}
          edges {{
            node {{
              name
              owner {{
                login
              }}
              id
              description
              diskUsage
              url
              sshUrl
              forkCount
              hasWikiEnabled
              homepageUrl
              isInOrganization
              isEmpty
              stargazerCount
              visibility
              isFork
              openGraphImageUrl
            }}
          }}
        }}
      }}
    }}
    """


def generate_markdown_report_user(data: Dict[str, Any], username: str, client=None) -> str:
    """Generate markdown report for user"""
    user = data.get('data', {}).get('user')
    if not user:
        return ""
    
    md_lines = []
    md_lines.append(f"# User: {username}")
    md_lines.append(f"**GitHub Profile:** [{username}](https://github.com/{username})")
    md_lines.append("")
    
    # User Information Table
    md_lines.append("## User Information")
    md_lines.append("")
    md_lines.append("| Field | Value |")
    md_lines.append("|-------|-------|")
    md_lines.append(f"| Login | {user.get('login', 'N/A')} |")
    md_lines.append(f"| Email | {user.get('email', 'N/A') or 'N/A'} |")
    md_lines.append(f"| Location | {user.get('location', 'N/A') or 'N/A'} |")
    md_lines.append(f"| Company | {user.get('company', 'N/A') or 'N/A'} |")
    following = user.get('following', {})
    following_count = following.get('totalCount', 0)
    md_lines.append(f"| Following | {following_count} |")
    
    # Following users list
    following_nodes = following.get('nodes', [])
    if following_nodes:
        md_lines.append("")
        md_lines.append("### Following Users")
        md_lines.append("")
        md_lines.append("| Login | Name | Email | Company | Location |")
        md_lines.append("|-------|------|-------|---------|----------|")
        for follow_user in following_nodes:
            login = follow_user.get('login', 'N/A')
            login_link = f"[{login}](https://github.com/{login})" if login != 'N/A' else 'N/A'
            md_lines.append(f"| {login_link} | {follow_user.get('name', 'N/A') or 'N/A'} | {follow_user.get('email', 'N/A') or 'N/A'} | {follow_user.get('company', 'N/A') or 'N/A'} | {follow_user.get('location', 'N/A') or 'N/A'} |")
        md_lines.append("")
    md_lines.append(f"| Followers | {user.get('followers', {}).get('totalCount', 0)} |")
    md_lines.append(f"| Total Repositories | {user.get('repositories', {}).get('totalCount', 0)} |")
    md_lines.append(f"| Total Gists | {user.get('gists', {}).get('totalCount', 0)} |")
    md_lines.append(f"| Total Gist Comments | {user.get('gistComments', {}).get('totalCount', 0)} |")
    md_lines.append("")
    
    # Repositories
    repos = user.get('repositories', {}).get('edges', [])
    if repos:
        md_lines.append("## Repositories")
        md_lines.append("")
        md_lines.append("| Repository Name | Description | Stars | Forks | Is Fork | URL |")
        md_lines.append("|----------------|-------------|-------|-------|---------|-----|")
        for repo_edge in repos:
            repo = repo_edge.get('node', {})
            repo_name = repo.get('name', 'N/A')
            repo_owner = repo.get('owner', {}).get('login', username)
            repo_url = repo.get('url', '#')
            repo_link = f"[{repo_name}]({repo_url})"
            description = (repo.get('description', 'N/A') or 'N/A')[:50]  # Truncate long descriptions
            md_lines.append(f"| {repo_link} | {description} | {repo.get('stargazerCount', 0)} | {repo.get('forkCount', 0)} | {repo.get('isFork', False)} | [{repo_url}]({repo_url}) |")
        md_lines.append("")
        
        # Clone URLs
        md_lines.append("### Repository Clone URLs (HTTPS)")
        md_lines.append("")
        for repo_edge in repos:
            repo = repo_edge.get('node', {})
            repo_url = repo.get('url', '')
            if repo_url:
                md_lines.append(f"- `{repo_url}.git`")
        md_lines.append("")
    
    # Gists
    gists = user.get('gists', {}).get('edges', [])
    if gists:
        md_lines.append("## Gists")
        md_lines.append("")
        md_lines.append("| ID | Name | Description | Public | Fork | Stars | Created | Updated | Pushed | URL |")
        md_lines.append("|----|------|-------------|--------|------|-------|---------|---------|--------|-----|")
        for gist_edge in gists:
            gist = gist_edge.get('node', {})
            gist_id = gist.get('id', 'N/A')
            gist_name = gist.get('name', 'N/A')
            gist_url = gist.get('url', '#')
            gist_link = f"[{gist_name}]({gist_url})"
            description = (gist.get('description', 'N/A') or 'N/A')[:50]
            is_public = gist.get('isPublic', False)
            is_fork = gist.get('isFork', False)
            star_count = gist.get('stargazerCount', 0)
            created = gist.get('createdAt', 'N/A')
            updated = gist.get('updatedAt', 'N/A')
            pushed = gist.get('pushedAt', 'N/A') or 'N/A'
            md_lines.append(f"| `{gist_id[:8]}` | {gist_link} | {description} | {is_public} | {is_fork} | {star_count} | {created} | {updated} | {pushed} | [{gist_url}]({gist_url}) |")
        md_lines.append("")
        
        # Detailed gist information in expandable sections
        for gist_edge in gists:
            gist = gist_edge.get('node', {})
            gist_name = gist.get('name', 'N/A')
            gist_url = gist.get('url', '#')
            gist_link = f"[{gist_name}]({gist_url})"
            
            md_lines.append(f"### {gist_link} - Details")
            md_lines.append("")
            
            # Gist metadata table
            md_lines.append("| Field | Value |")
            md_lines.append("|-------|-------|")
            md_lines.append(f"| ID | `{gist.get('id', 'N/A')}` |")
            md_lines.append(f"| Name | {gist.get('name', 'N/A')} |")
            md_lines.append(f"| Description | {gist.get('description', 'N/A') or 'N/A'} |")
            md_lines.append(f"| Public | {gist.get('isPublic', False)} |")
            md_lines.append(f"| Is Fork | {gist.get('isFork', False)} |")
            md_lines.append(f"| Resource Path | {gist.get('resourcePath', 'N/A')} |")
            md_lines.append(f"| Created | {gist.get('createdAt', 'N/A')} |")
            md_lines.append(f"| Updated | {gist.get('updatedAt', 'N/A')} |")
            md_lines.append(f"| Pushed | {gist.get('pushedAt', 'N/A') or 'N/A'} |")
            md_lines.append(f"| Stargazer Count | {gist.get('stargazerCount', 0)} |")
            md_lines.append(f"| Viewer Has Starred | {gist.get('viewerHasStarred', False)} |")
            
            # Owner
            owner = gist.get('owner', {})
            if owner:
                owner_login = owner.get('login', 'N/A')
                md_lines.append(f"| Owner | [{owner_login}](https://github.com/{owner_login}) |")
            
            # Zip download
            if gist_url and '/gist.github.com/' in gist_url:
                # Append /archive/main.zip to the gist URL
                zip_url = f"{gist_url}/archive/main.zip"
                md_lines.append(f"| Download ZIP | [{zip_url}]({zip_url}) |")
            md_lines.append("")
            
            # Files table
            files = gist.get('files', [])
            if files:
                md_lines.append("#### Files")
                md_lines.append("")
                md_lines.append("| Name | Encoded Name | Extension | Language | Size | Encoding | Is Image | Truncated |")
                md_lines.append("|------|--------------|-----------|----------|------|----------|----------|-----------|")
                for file in files:
                    file_name = file.get('name', 'N/A')
                    encoded_name = file.get('encodedName', 'N/A')
                    extension = file.get('extension', 'N/A') or 'N/A'
                    language = file.get('language', {}).get('name', 'N/A') or 'N/A'
                    size = file.get('size', 0) or 0
                    encoding = file.get('encoding', 'N/A') or 'N/A'
                    is_image = file.get('isImage', False)
                    is_truncated = file.get('isTruncated', False)
                    md_lines.append(f"| {file_name} | {encoded_name} | {extension} | {language} | {size} | {encoding} | {is_image} | {is_truncated} |")
                md_lines.append("")
            
            # Comments table
            comments = gist.get('comments', {})
            if comments:
                comment_count = comments.get('totalCount', 0)
                if comment_count > 0:
                    md_lines.append(f"#### Comments ({comment_count})")
                    md_lines.append("")
                    comment_nodes = comments.get('nodes', [])
                    if comment_nodes:
                        md_lines.append("| Author | Created | Updated | Preview |")
                        md_lines.append("|--------|--------|---------|---------|")
                        for comment in comment_nodes[:20]:  # Show first 20
                            author = comment.get('author', {})
                            author_login = author.get('login', 'N/A')
                            author_url = author.get('url', '#')
                            author_link = f"[{author_login}]({author_url})" if author_login != 'N/A' else 'N/A'
                            body_preview = (comment.get('bodyText', 'N/A') or 'N/A')[:100].replace('\n', ' ')
                            md_lines.append(f"| {author_link} | {comment.get('createdAt', 'N/A')} | {comment.get('updatedAt', 'N/A')} | {body_preview}... |")
                md_lines.append("")
            
            # Stargazers table
            stargazers = gist.get('stargazers', {})
            if stargazers:
                star_count = stargazers.get('totalCount', 0)
                if star_count > 0:
                    md_lines.append(f"#### Stargazers ({star_count})")
                    md_lines.append("")
                    star_nodes = stargazers.get('nodes', [])
                    if star_nodes:
                        md_lines.append("| Login | Name | Email | URL |")
                        md_lines.append("|-------|------|-------|-----|")
                        for star in star_nodes[:20]:  # Show first 20
                            star_login = star.get('login', 'N/A')
                            star_url = star.get('url', '#')
                            star_link = f"[{star_login}]({star_url})" if star_login != 'N/A' else 'N/A'
                            md_lines.append(f"| {star_link} | {star.get('name', 'N/A') or 'N/A'} | {star.get('email', 'N/A') or 'N/A'} | [{star_url}]({star_url}) |")
                md_lines.append("")
            
            # Forks table
            forks = gist.get('forks', {})
            if forks:
                fork_count = forks.get('totalCount', 0)
                if fork_count > 0:
                    md_lines.append(f"#### Forks ({fork_count})")
                    md_lines.append("")
                    fork_nodes = forks.get('nodes', [])
                    if fork_nodes:
                        md_lines.append("| Name | Owner | URL |")
                        md_lines.append("|------|-------|-----|")
                        for fork in fork_nodes[:20]:  # Show first 20
                            fork_name = fork.get('name', 'N/A')
                            fork_url = fork.get('url', '#')
                            fork_owner = fork.get('owner', {}).get('login', 'N/A')
                            md_lines.append(f"| {fork_name} | [{fork_owner}](https://github.com/{fork_owner}) | [{fork_url}]({fork_url}) |")
                md_lines.append("")
            
            md_lines.append("---")
            md_lines.append("")
    
    # Gist Comments
    gist_comments = user.get('gistComments', {}).get('nodes', [])
    if gist_comments:
        md_lines.append("## Gist Comments")
        md_lines.append("")
        md_lines.append("| Gist URL | Created | Updated | Preview |")
        md_lines.append("|----------|--------|---------|---------|")
        for comment in gist_comments:
            gist_url = comment.get('gist', {}).get('url', '#')
            body_preview = (comment.get('body', 'N/A') or 'N/A')[:100].replace('\n', ' ')
            md_lines.append(f"| [{gist_url}]({gist_url}) | {comment.get('createdAt', 'N/A')} | {comment.get('updatedAt', 'N/A')} | {body_preview}... |")
        md_lines.append("")
    
    return "\n".join(md_lines)


def print_user_info(data: Dict[str, Any], username: str, client=None, print_gists: bool = False) -> None:
    """Print formatted user information"""
    user = data.get('data', {}).get('user')
    if not user:
        print("Error: User not found or not accessible")
        return
    
    print(f"\n[+] User Enumeration: {username}")
    print(f"Login: {user.get('login')}")
    print(f"Email: {user.get('email', 'N/A')}")
    print(f"Location: {user.get('location', 'N/A')}")
    print(f"Company: {user.get('company', 'N/A')}")
    following = user.get('following', {})
    print(f"Following: {following.get('totalCount', 0)}")
    following_nodes = following.get('nodes', [])
    if following_nodes:
        print("  Users being followed:")
        for follow_user in following_nodes[:20]:  # Show first 20
            print(f"    - {follow_user.get('login', 'N/A')} ({follow_user.get('url', 'N/A')})")
    print(f"Followers: {user.get('followers', {}).get('totalCount', 0)}")
    print(f"Total Repositories: {user.get('repositories', {}).get('totalCount', 0)}")
    print(f"Total Gists: {user.get('gists', {}).get('totalCount', 0)}")
    print(f"Total Gist Comments: {user.get('gistComments', {}).get('totalCount', 0)}")
    
    repos = user.get('repositories', {}).get('edges', [])
    print(f"\n[+] Repositories for {username}: {user.get('repositories', {}).get('totalCount', 0)}")
    for repo_edge in repos:
        repo = repo_edge.get('node', {})
        repo_name = repo.get('name', 'N/A')
        description = repo.get('description') or 'None'
        disk_usage = repo.get('diskUsage', 'N/A')
        url = repo.get('url', 'N/A')
        homepage = repo.get('homepageUrl') or 'None'
        wiki = repo.get('hasWikiEnabled', False)
        stars = repo.get('stargazerCount', 0)
        in_org = repo.get('isInOrganization', False)
        fork_count = repo.get('forkCount', 0)
        is_fork = repo.get('isFork', False)
        is_empty = repo.get('isEmpty', False)
        
        # Format: Name | Description | Stats
        stats_parts = []
        if stars > 0:
            stats_parts.append(f"⭐ {stars}")
        if fork_count > 0:
            stats_parts.append(f"🍴 {fork_count}")
        if is_fork:
            stats_parts.append("Fork")
        if is_empty:
            stats_parts.append("Empty")
        stats_str = " | ".join(stats_parts) if stats_parts else "No stats"
        
        print(f"  {repo_name}")
        print(f"    Description: {description}")
        print(f"    URL: {url}")
        if homepage != 'None':
            print(f"    Homepage: {homepage}")
        print(f"    Disk: {disk_usage} | Wiki: {wiki} | Org: {in_org} | {stats_str}")
    
    gists = user.get('gists', {}).get('edges', [])
    gist_count = user.get('gists', {}).get('totalCount', 0)
    print(f"\n[+] Gists from {username}: {gist_count}")
    
    if gists:
        print("")
        # Summary table
        print(f"{'ID':<12} {'Name':<30} {'Public':<8} {'Fork':<6} {'Stars':<6} {'Created':<20} {'URL':<50}")
        print("-" * 150)
        for gist_edge in gists:
            gist = gist_edge.get('node', {})
            gist_id = gist.get('id', 'N/A')[:12]
            gist_name = gist.get('name', 'N/A')[:28]
            is_public = str(gist.get('isPublic', False))
            is_fork = str(gist.get('isFork', False))
            star_count = gist.get('stargazerCount', 0)
            created = gist.get('createdAt', 'N/A')[:18]
            gist_url = gist.get('url', 'N/A')[:48]
            print(f"{gist_id:<12} {gist_name:<30} {is_public:<8} {is_fork:<6} {star_count:<6} {created:<20} {gist_url:<50}")
        print("")
        
        # Detailed info for each gist
        for gist_edge in gists:
            gist = gist_edge.get('node', {})
            print(f"\n[+] Gist: {gist.get('name', 'N/A')}")
            print(f"  ID: {gist.get('id', 'N/A')}")
            print(f"  Description: {gist.get('description', 'N/A')}")
            print(f"  URL: {gist.get('url', 'N/A')}")
            print(f"  Resource Path: {gist.get('resourcePath', 'N/A')}")
            print(f"  Public: {gist.get('isPublic', False)}")
            print(f"  Is Fork: {gist.get('isFork', False)}")
            print(f"  Created: {gist.get('createdAt', 'N/A')}")
            print(f"  Updated: {gist.get('updatedAt', 'N/A')}")
            print(f"  Pushed: {gist.get('pushedAt', 'N/A') or 'N/A'}")
            print(f"  Stargazer Count: {gist.get('stargazerCount', 0)}")
            print(f"  Viewer Has Starred: {gist.get('viewerHasStarred', False)}")
            
            owner = gist.get('owner', {})
            if owner:
                print(f"  Owner: {owner.get('login', 'N/A')} ({owner.get('id', 'N/A')})")
            
            # Files
            files = gist.get('files', [])
            if files:
                print(f"  Files ({len(files)}):")
                print(f"    {'Name':<30} {'Extension':<12} {'Language':<15} {'Size':<10} {'Encoding':<10} {'Image':<8} {'Truncated':<10}")
                print(f"    {'-'*30} {'-'*12} {'-'*15} {'-'*10} {'-'*10} {'-'*8} {'-'*10}")
                for file in files:
                    file_name = (file.get('name', 'N/A') or file.get('encodedName', 'N/A'))[:28]
                    extension = (file.get('extension', 'N/A') or 'N/A')[:10]
                    language = (file.get('language', {}).get('name', 'N/A') or 'N/A')[:13]
                    size = str(file.get('size', 0) or 0)[:8]
                    encoding = (file.get('encoding', 'N/A') or 'N/A')[:8]
                    is_image = str(file.get('isImage', False))[:6]
                    is_truncated = str(file.get('isTruncated', False))[:8]
                    print(f"    {file_name:<30} {extension:<12} {language:<15} {size:<10} {encoding:<10} {is_image:<8} {is_truncated:<10}")
                    
                    # Display file content for plain text files
                    file_text = file.get('text')
                    if file_text and not file.get('isImage', False) and not file.get('isTruncated', False):
                        print(f"    Content:")
                        # Split into lines and indent each line
                        content_lines = file_text.split('\n')
                        for line in content_lines[:100]:  # Limit to first 100 lines to avoid huge output
                            print(f"      {line}")
                        if len(content_lines) > 100:
                            print(f"      ... ({len(content_lines) - 100} more lines)")
                        print()
            
            # Comments
            comments = gist.get('comments', {})
            if comments:
                comment_count = comments.get('totalCount', 0)
                print(f"  Comments: {comment_count}")
                comment_nodes = comments.get('nodes', [])
                if comment_nodes:
                    for comment in comment_nodes[:5]:  # Show first 5
                        author = comment.get('author', {})
                        print(f"    - {author.get('login', 'N/A')}: {comment.get('bodyText', 'N/A')[:60]}... ({comment.get('createdAt', 'N/A')})")
            
            # Stargazers
            stargazers = gist.get('stargazers', {})
            if stargazers:
                star_count = stargazers.get('totalCount', 0)
                print(f"  Stargazers: {star_count}")
                star_nodes = stargazers.get('nodes', [])
                if star_nodes:
                    star_logins = [s.get('login', '') for s in star_nodes[:10]]
                    print(f"    Top: {', '.join(star_logins)}")
            
            # Forks
            forks = gist.get('forks', {})
            if forks:
                fork_count = forks.get('totalCount', 0)
                print(f"  Forks: {fork_count}")
                fork_nodes = forks.get('nodes', [])
                if fork_nodes:
                    for fork in fork_nodes[:5]:  # Show first 5
                        fork_owner = fork.get('owner', {}).get('login', 'N/A')
                        print(f"    - {fork_owner}/{fork.get('name', 'N/A')}: {fork.get('url', 'N/A')}")
            
            # Zip download
            gist_url = gist.get('url', '')
            if gist_url and '/gist.github.com/' in gist_url:
                # Append /archive/main.zip to the gist URL
                zip_url = f"{gist_url}/archive/main.zip"
                print(f"  Download ZIP: {zip_url}")
            
            print("---")
    
    gist_comments = user.get('gistComments', {}).get('nodes', [])
    print(f"\n[+] Gist Comments from {username}: {user.get('gistComments', {}).get('totalCount', 0)} \n")
    for comment in gist_comments:
        print(f"Gist Comment: {comment.get('gist', {}).get('url')}")
        print(f"Created: {comment.get('createdAt')}")
        print(f"Updated: {comment.get('updatedAt')}")
        print(f"Body: {comment.get('body', 'N/A')[:200]}...")
        print("---")
    
    print(f"\n[+] END {username}\n")

