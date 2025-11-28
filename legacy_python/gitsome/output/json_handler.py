"""JSON output handler"""

import os
import json
from typing import Dict, Any


def save_json(data: Dict[str, Any], filename: str) -> str:
    """Save JSON data to file in output/ folder"""
    # Ensure output directory exists
    output_dir = "output"
    os.makedirs(output_dir, exist_ok=True)
    
    filepath = os.path.join(output_dir, filename)
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
    print(f"[*] Saved JSON output to: {filepath}")
    return filepath

