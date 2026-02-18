#!/usr/bin/env python3
"""Resolve client entries: ensure each client has an rpass API key, then output JSON.

Takes a path to inventory.yaml as a CLI argument, generates missing API keys
via rpass, and prints a JSON array of {id, api_key} objects to stdout.
"""

import json
import subprocess
import sys
from pathlib import Path

import yaml

RPASS_PREFIX = "personal/s3uploader/clients"


def rpass_has(target: str) -> bool:
    result = subprocess.run(["rpass", "has", target], capture_output=True, text=True, check=True)
    return result.stdout.strip() == "1"


def rpass_gen(target: str) -> None:
    subprocess.run(["rpass", "gen", target], capture_output=True, check=True)


def rpass_get(target: str) -> str:
    result = subprocess.run(["rpass", "get", target], capture_output=True, text=True, check=True)
    return result.stdout.strip()


def main():
    inventory_path = Path(sys.argv[1]).expanduser()
    inventory = yaml.safe_load(inventory_path.read_text())

    hosts = inventory.get("all", {}).get("children", {}).get("servers", {}).get("hosts", {})

    entries = []
    for hostname, config in hosts.items():
        if "sync_watches" not in config:
            continue
        target = f"{RPASS_PREFIX}/{hostname}"
        if not rpass_has(target):
            rpass_gen(target)
        entries.append({"id": hostname, "api_key": rpass_get(target)})

    print(json.dumps(entries))


if __name__ == "__main__":
    main()
