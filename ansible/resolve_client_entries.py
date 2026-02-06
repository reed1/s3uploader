#!/usr/bin/env python3
"""Resolve client entries: ensure each client has an rpass API key, then output JSON.

Reads clients.yaml, generates missing API keys via rpass, and prints
a JSON array of {id, api_key} objects to stdout.
"""

import json
import subprocess
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
    clients_path = Path(__file__).parent / "clients.yaml"
    clients = yaml.safe_load(clients_path.read_text())["clients"]

    entries = []
    for client in clients:
        client_id = client["id"]
        target = f"{RPASS_PREFIX}/{client_id}"
        if not rpass_has(target):
            rpass_gen(target)
        entries.append({"id": client_id, "api_key": rpass_get(target)})

    print(json.dumps(entries))


if __name__ == "__main__":
    main()
