"""GitHub GraphQL API Client"""

import requests
from typing import Dict, Any


class GitHubGraphQLClient:
    """Client for GitHub GraphQL API"""
    
    def __init__(self, token: str):
        self.token = token
        self.api_url = "https://api.github.com/graphql"
        self.headers = {
            "Authorization": f"token {token}",
            "Content-Type": "application/json"
        }
    
    def query(self, query: str) -> Dict[str, Any]:
        """Execute a GraphQL query"""
        payload = {"query": query}
        response = requests.post(self.api_url, headers=self.headers, json=payload)
        response.raise_for_status()
        data = response.json()
        
        # Check for GraphQL errors (GraphQL can return 200 with errors in response)
        if 'errors' in data:
            error_messages = [err.get('message', 'Unknown error') for err in data['errors']]
            raise Exception(f"GraphQL errors: {'; '.join(error_messages)}")
        
        return data
    
    def compare_branches(self, owner: str, repo_name: str, base_ref: str, head_ref: str) -> Dict[str, Any]:
        """Compare two branches to see how many commits ahead/behind"""
        # Ensure refs are in the correct format
        base_ref_formatted = base_ref if base_ref.startswith('refs/') else f"refs/heads/{base_ref}"
        head_ref_formatted = head_ref if head_ref.startswith('refs/') else f"refs/heads/{head_ref}"
        
        query = f"""
        query {{
          repository(owner: "{owner}", name: "{repo_name}") {{
            compare(baseRef: "{base_ref_formatted}", headRef: "{head_ref_formatted}") {{
              aheadBy
              behindBy
            }}
          }}
        }}
        """
        data = self.query(query)
        return data.get('data', {}).get('repository', {}).get('compare', {})
    
    def get_gist_latest_commit_sha(self, gist_id: str) -> str:
        """Get the latest commit SHA for a gist using REST API"""
        rest_headers = {
            "Authorization": f"token {self.token}",
            "Accept": "application/vnd.github.v3+json"
        }
        rest_url = f"https://api.github.com/gists/{gist_id}"
        try:
            response = requests.get(rest_url, headers=rest_headers)
            response.raise_for_status()
            data = response.json()
            # Get latest commit SHA from history
            history = data.get('history', [])
            if history:
                return history[0].get('version', '')
        except Exception:
            pass
        return ''

