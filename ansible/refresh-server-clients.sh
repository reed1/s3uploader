#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
ansible-playbook -i inventory.yaml setup-server.yaml --tags clients "$@"
