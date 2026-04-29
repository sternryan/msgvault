#!/usr/bin/env bash
# run-triage.sh — weekly Monday 07:00 PT cron wrapper for msgvault triage +
# digest send. Loads /opt/services/msgvault/.env explicitly; intentionally
# does NOT source any user shell rc file (memory:
# project_mac_mini_launchd_gotchas.md — sourcing user rc clobbers the env
# loaded from .env above).

set -euo pipefail

# Explicit .env load — every var defined in /opt/services/msgvault/.env is
# exported into the environment of the msgvault triage + digest send calls.
set -a
[ -f /opt/services/msgvault/.env ] && . /opt/services/msgvault/.env
set +a

# Required env (set in .env at deploy time):
#   DIGEST_TO_ADDR       — recipient email
#   DIGEST_FROM_ADDR     — sender email (typically same as DIGEST_TO_ADDR)
#   MSGVAULT_ACCOUNT     — msgvault account whose OAuth token sends
#   FORGE_GRAPH_DB       — path to forge graph.db (read-only)
#   FORGE_SOURCES_DB     — path to forge sources.db (read-only)
#   USER_PRIMARY_EMAIL   — user's primary address for curiosity/decision scoring

OUT="/opt/services/msgvault/runs/triage-$(date +%Y%m%d).jsonl"
mkdir -p "$(dirname "$OUT")"
mkdir -p /opt/services/msgvault/logs

/opt/services/msgvault/bin/msgvault triage \
    --since 7d \
    --out "$OUT" \
    --forge-graph "${FORGE_GRAPH_DB:-/opt/services/forge/graph.db}" \
    --forge-sources "${FORGE_SOURCES_DB:-/opt/services/forge/sources.db}" \
    --user-email "${USER_PRIMARY_EMAIL:-}"

/opt/services/msgvault/bin/msgvault digest send \
    --in "$OUT" \
    --to "$DIGEST_TO_ADDR" \
    --from "${DIGEST_FROM_ADDR:-$DIGEST_TO_ADDR}" \
    --account "$MSGVAULT_ACCOUNT"
