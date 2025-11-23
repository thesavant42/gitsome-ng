#!/usr/bin/env python3
"""
gitsome - GitHub Info Enumerator
Unified Python script for querying GitHub via GraphQL API
Supports organization, repository, and user enumeration
"""

import os
import sys
import argparse
from datetime import datetime

try:
    import requests
except ImportError:
    print("Error: requests library is required. Install with: pip install requests", file=sys.stderr)
    sys.exit(1)

# Import from modular structure
from gitsome.constants import BANNER
from gitsome.client import GitHubGraphQLClient
from gitsome.output.json_handler import save_json
from gitsome.output.markdown_handler import save_markdown_report
from gitsome.handlers.org_handler import (
    get_org_query,
    generate_markdown_report_org,
    print_org_info
)
from gitsome.handlers.repo_handler import (
    get_repo_query,
    build_branch_comparison_query,
    generate_markdown_report,
    print_repo_info
)
from gitsome.handlers.user_handler import (
    get_user_query,
    generate_markdown_report_user,
    print_user_info
)


def main():
    # Print banner immediately when script launches
    print(BANNER)
    
    parser = argparse.ArgumentParser(
        description="GitHub Info Enumerator - Query GitHub via GraphQL API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s org thesavant42
  %(prog)s repo facebook graphql
  %(prog)s user octocat
        """
    )
    
    subparsers = parser.add_subparsers(dest='command', help='Command to execute')
    
    # Organization command
    org_parser = subparsers.add_parser('org', help='Query organization repositories')
    org_parser.add_argument('org_name', nargs='?', default='thesavant42',
                           help='Organization name (default: thesavant42)')
    org_parser.add_argument('--no-save', action='store_true',
                           help='Do not save JSON output to file')
    
    # Repository command
    repo_parser = subparsers.add_parser('repo', help='Query repository details')
    repo_parser.add_argument('owner', nargs='?', default='thesavant42',
                            help='Repository owner (default: thesavant42)')
    repo_parser.add_argument('repo_name', nargs='?', default='gitsome',
                            help='Repository name (default: gitsome)')
    repo_parser.add_argument('--no-save', action='store_true',
                            help='Do not save JSON output to file')
    
    # User command
    user_parser = subparsers.add_parser('user', help='Query user details')
    user_parser.add_argument('username', help='GitHub username')
    user_parser.add_argument('--no-save', action='store_true',
                           help='Do not save JSON output to file')
    user_parser.add_argument('--print-gists', action='store_true',
                           help='Print detailed gist information')
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        sys.exit(1)
    
    # Check for GitHub token
    token = os.environ.get('GITHUB_TOKEN')
    if not token:
        print("Error: Must provide GITHUB_TOKEN in environment", file=sys.stderr)
        sys.exit(1)
    
    client = GitHubGraphQLClient(token)
    
    # Print module-specific header based on command
    if args.command == 'org':
        print("Repository Enumeration Module\n")
    elif args.command == 'repo':
        print("Repository Enumeration Module\n")
    elif args.command == 'user':
        print(f"gitSome - gitHub Info Enumerator, by savant42 - {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
        print("User Enumeration Module")
    
    try:
        if args.command == 'org':
            query = get_org_query(args.org_name)
            data = client.query(query)
            
            if not args.no_save:
                save_json(data, f"{args.org_name}-org.json")
                # Generate markdown report
                markdown_content = generate_markdown_report_org(data, args.org_name)
                save_markdown_report(markdown_content, f"{args.org_name}-org.md")
            
            print_org_info(data, args.org_name)
            
        elif args.command == 'repo':
            query = get_repo_query(args.owner, args.repo_name)
            data = client.query(query)
            
            # Get default branch and all branch names for comparison query
            repo_data = data.get('data', {}).get('repository', {})
            default_branch = repo_data.get('defaultBranchRef', {}).get('name', 'master')
            branch_nodes = repo_data.get('refs', {}).get('nodes', [])
            
            # Build a single query with aliases for all branch comparisons
            if branch_nodes and repo_data.get('isFork'):
                branch_names = [b.get('name') for b in branch_nodes if b.get('name') != default_branch]
                if branch_names:
                    try:
                        comparison_query = build_branch_comparison_query(args.owner, args.repo_name, default_branch, branch_names)
                        comparison_data = client.query(comparison_query)
                        # Merge comparison data into main data - map aliases back to branch names
                        comparisons = comparison_data.get('data', {}).get('repository', {})
                        for i, branch_name in enumerate(branch_names[:50]):
                            alias = f"ref_{i}".replace('-', '_').replace('.', '_')
                            if alias in comparisons:
                                # Get the compare data from the ref object
                                ref_data = comparisons[alias]
                                if ref_data and 'compare' in ref_data:
                                    # Find the branch node and add comparison data
                                    for branch in branch_nodes:
                                        if branch.get('name') == branch_name:
                                            branch['compare'] = ref_data['compare']
                                            break
                    except Exception as e:
                        print(f"  [DEBUG] Error fetching branch comparisons: {str(e)}", file=sys.stderr)
            
            if not args.no_save:
                save_json(data, f"{args.owner}-{args.repo_name}-repo.json")
                # Generate markdown report
                markdown_content = generate_markdown_report(data, args.owner, args.repo_name)
                save_markdown_report(markdown_content, f"{args.owner}-{args.repo_name}-repo.md")
            
            print_repo_info(data, args.owner, args.repo_name, client)
            
        elif args.command == 'user':
            query = get_user_query(args.username)
            data = client.query(query)
            
            if not args.no_save:
                save_json(data, f"{args.username}-user.json")
                # Generate markdown report
                markdown_content = generate_markdown_report_user(data, args.username)
                save_markdown_report(markdown_content, f"{args.username}-user.md")
            
            print_user_info(data, args.username, client, print_gists=args.print_gists)
            
    except requests.exceptions.HTTPError as e:
        print(f"Error: HTTP {e.response.status_code}", file=sys.stderr)
        if e.response.status_code == 401:
            print("Authentication failed. Check your GITHUB_TOKEN.", file=sys.stderr)
        elif e.response.status_code == 403:
            print("Rate limit exceeded or forbidden. Check your token permissions.", file=sys.stderr)
        try:
            error_data = e.response.json()
            if 'errors' in error_data:
                for error in error_data['errors']:
                    print(f"  {error.get('message', 'Unknown error')}", file=sys.stderr)
        except:
            pass
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

