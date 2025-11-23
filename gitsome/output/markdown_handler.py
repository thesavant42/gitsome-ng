"""Markdown output handler"""

import os


def save_markdown_report(markdown_content: str, filename: str) -> None:
    """Save markdown report to file in output/ folder"""
    if not markdown_content:
        return
    
    # Ensure output directory exists
    output_dir = "output"
    os.makedirs(output_dir, exist_ok=True)
    
    filepath = os.path.join(output_dir, filename)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(markdown_content)
    print(f"[*] Saved Markdown report to: {filepath}")

