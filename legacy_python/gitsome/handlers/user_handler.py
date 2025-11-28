"""User handler - query, format, and report generation for users"""

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


def generate_markdown_report_user(data: Dict[str, Any], username: str) -> str:
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
    # Fix: safely handle None values
    followers_obj = user.get('followers') or {}
    md_lines.append(f"| Followers | {followers_obj.get('totalCount', 0) if isinstance(followers_obj, dict) else 0} |")
    repos_obj = user.get('repositories') or {}
    md_lines.append(f"| Total Repositories | {repos_obj.get('totalCount', 0) if isinstance(repos_obj, dict) else 0} |")
    gists_obj = user.get('gists') or {}
    md_lines.append(f"| Total Gists | {gists_obj.get('totalCount', 0) if isinstance(gists_obj, dict) else 0} |")
    gist_comments_obj = user.get('gistComments') or {}
    md_lines.append(f"| Total Gist Comments | {gist_comments_obj.get('totalCount', 0) if isinstance(gist_comments_obj, dict) else 0} |")
    md_lines.append("")
    
    # Repositories
    repos = repos_obj.get('edges', []) if isinstance(repos_obj, dict) else []
    if repos:
        md_lines.append("## Repositories")
        md_lines.append("")
        md_lines.append("| Repository Name | Description | Stars | Forks | Is Fork | URL |")
        md_lines.append("|----------------|-------------|-------|-------|---------|-----|")
        for repo_edge in repos:
            repo = repo_edge.get('node', {})
            repo_name = repo.get('name', 'N/A')
            # Fix: safely handle None owner
            owner_obj = repo.get('owner') or {}
            repo_owner = owner_obj.get('login', username) if isinstance(owner_obj, dict) else username
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
    gists_obj = user.get('gists') or {}
    gists = gists_obj.get('edges', []) if isinstance(gists_obj, dict) else []
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
                    # Fix: safely handle None language
                    language_obj = file.get('language')
                    language = (language_obj or {}).get('name', 'N/A') if language_obj else 'N/A'
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
                            # Fix: safely handle None owner
                            fork_owner_obj = fork.get('owner') or {}
                            fork_owner = fork_owner_obj.get('login', 'N/A') if isinstance(fork_owner_obj, dict) else 'N/A'
                            md_lines.append(f"| {fork_name} | [{fork_owner}](https://github.com/{fork_owner}) | [{fork_url}]({fork_url}) |")
                md_lines.append("")
            
            md_lines.append("---")
            md_lines.append("")
    
    # Gist Comments
    gist_comments_obj = user.get('gistComments') or {}
    gist_comments = gist_comments_obj.get('nodes', []) if isinstance(gist_comments_obj, dict) else []
    if gist_comments:
        md_lines.append("## Gist Comments")
        md_lines.append("")
        md_lines.append("| Gist URL | Created | Updated | Preview |")
        md_lines.append("|----------|--------|---------|---------|")
        for comment in gist_comments:
            # Fix: safely handle None gist
            gist_obj = comment.get('gist')
            gist_url = (gist_obj or {}).get('url', '#') if gist_obj else '#'
            body_preview = (comment.get('body', 'N/A') or 'N/A')[:100].replace('\n', ' ')
            md_lines.append(f"| [{gist_url}]({gist_url}) | {comment.get('createdAt', 'N/A')} | {comment.get('updatedAt', 'N/A')} | {body_preview}... |")
        md_lines.append("")
    
    return "\n".join(md_lines)


def print_user_info(data: Dict[str, Any], username: str) -> None:
    """Print formatted user information matching bash script format"""
    user = data.get('data', {}).get('user')
    if not user:
        print("Error: User not found or not accessible")
        return
    
    # Basic user info - matching bash script lines 141-149
    print(f"\n[+] User Enumeration: {username}")
    print(f"Login: {user.get('login', 'N/A')}")
    print(f"Email: {user.get('email', 'N/A') or 'N/A'}")
    print(f"Location: {user.get('location', 'N/A') or 'N/A'}")
    print(f"Company: {user.get('company', 'N/A') or 'N/A'}")
    following = user.get('following', {}) or {}
    print(f"Following: {following.get('totalCount', 0)}")
    followers = user.get('followers', {}) or {}
    print(f"Followers: {followers.get('totalCount', 0)}")
    repos_total = user.get('repositories', {}).get('totalCount', 0) if user.get('repositories') else 0
    print(f"Total Repositories: {repos_total}")
    gists_total = user.get('gists', {}).get('totalCount', 0) if user.get('gists') else 0
    print(f"Total Gists: {gists_total}")
    gist_comments_total = user.get('gistComments', {}).get('totalCount', 0) if user.get('gistComments') else 0
    print(f"Total Gist Comments: {gist_comments_total}")
    
    # Repositories - matching bash script lines 152-166
    repos = user.get('repositories', {}).get('edges', [])
    print(f"\n[+] Repositories for {username}: {repos_total} \n---")
    for repo_edge in repos:
        repo = repo_edge.get('node', {})
        repo_name = repo.get('name', 'N/A')
        description = repo.get('description') or 'None'
        disk_usage = repo.get('diskUsage', 'N/A')
        url = repo.get('url', 'N/A')
        ssh_url = repo.get('sshUrl', 'N/A')
        homepage = repo.get('homepageUrl') or 'None'
        wiki = repo.get('hasWikiEnabled', False)
        stars = repo.get('stargazerCount', 0)
        in_org = repo.get('isInOrganization', False)
        fork_count = repo.get('forkCount', 0)
        is_fork = repo.get('isFork', False)
        is_empty = repo.get('isEmpty', False)
        
        print(f"Repository Name: {repo_name}")
        print(f"Description: {description}")
        print(f"Disk Usage: {disk_usage}")
        print(f"HTTPS URL: {url}")
        print(f"SSH URL: {ssh_url}")
        print(f"Homepage: {homepage}")
        print(f"GitWiki: {wiki}")
        print(f"Stargazer Count: {stars}")
        print(f"Is In Org: {in_org}")
        print(f"Fork Count: {fork_count}")
        print(f"Is Fork: {is_fork}")
        print(f"Is Empty: {is_empty}")
        print("---")
    
    # Clone URL list - matching bash script lines 169-170
    if repos:
        print(f"\n[+] Repos Clone URL List (HTTPS):\n---")
        for repo_edge in repos:
            repo = repo_edge.get('node', {})
            url = repo.get('url', '')
            if url:
                print(f"{url}.git")
    
    # Gists - matching bash script lines 173-181
    gists = user.get('gists', {}).get('edges', [])
    print(f"\n[+] Gists from {username}: {gists_total} \n")
    for gist_edge in gists:
        gist = gist_edge.get('node', {})
        gist_name = gist.get('name', 'N/A')
        gist_url = gist.get('url', 'N/A')
        description = gist.get('description') or 'None'
        
        # Get file information - bash script shows first file's info
        files = gist.get('files', [])
        if files:
            for file in files:
                encoded_name = file.get('encodedName', 'N/A')
                language = file.get('language', {})
                language_name = language.get('name', 'N/A') if language else 'N/A'
                size = file.get('size', 'N/A')
                
                print(f"Gist ID: {gist_name}")
                print(f"URL: {gist_url}")
                print(f"Filename: {encoded_name}")
                print(f"Language: {language_name}")
                print(f"Description: {description}")
                print(f"Size: {size}")
                print("---")
                break  # Only show first file per gist to match bash script format
        else:
            # No files, still show gist info
            print(f"Gist ID: {gist_name}")
            print(f"URL: {gist_url}")
            print(f"Filename: N/A")
            print(f"Language: N/A")
            print(f"Description: {description}")
            print(f"Size: N/A")
            print("---")
    
    # Gist Comments - matching bash script lines 184-190
    gist_comments = user.get('gistComments', {}).get('nodes', [])
    if gist_comments:
        print(f"\n[+] Gist Comments from {username}: {gist_comments_total} \n")
        for comment in gist_comments:
            # Fix: safely handle None gist
            gist_obj = comment.get('gist')
            gist_url = (gist_obj or {}).get('url', 'N/A') if gist_obj else 'N/A'
            created = comment.get('createdAt', 'N/A')
            updated = comment.get('updatedAt', 'N/A')
            body = comment.get('body', 'N/A') or 'N/A'
            
            print(f"Gist Comment: {gist_url}")
            print(f"Created: {created}")
            print(f"Updated: {updated}")
            print(f"Body: {body}")
            print("---")
    
    print(f"\n[+] END {username}\n")

