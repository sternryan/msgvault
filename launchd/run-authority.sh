#!/usr/bin/env bash
# run-authority.sh — daily 01:30 PT cron wrapper for msgvault authority
# graph recompute. Loads /opt/services/msgvault/.env explicitly;
# intentionally does NOT source any user shell rc file (memory:
# project_mac_mini_launchd_gotchas.md — sourcing user rc clobbers the
# env loaded from .env above).
#
# Mirrors run-triage.sh hygiene exactly:
#   - set -euo pipefail
#   - explicit set -a + . .env + set +a (no shell rc sourced)
#   - logs to launchd-managed StandardOut/ErrorPath via plist
#   - reentrant-safe; ThrottleInterval=30 enforced in plist

set -euo pipefail

# Explicit .env load — every var defined in /opt/services/msgvault/.env is
# exported into the environment of the msgvault authority recompute call.
set -a
[ -f /opt/services/msgvault/.env ] && . /opt/services/msgvault/.env
set +a

# Required env (set in .env at deploy time; defaults supplied below for
# the two forge paths but MSGVAULT_USER_EMAIL is operator-required because
# the is_from_me reply-detection fallback (A1 / Pitfall 3) needs it):
#   MSGVAULT_USER_EMAIL   — user's primary address
#   FORGE_SOURCES_DIR     — forge sources/ on disk (defaulted)
#   FORGE_SOURCES_DB      — forge sources.db path  (defaulted)

export FORGE_SOURCES_DIR="${FORGE_SOURCES_DIR:-/opt/services/forge/sources}"
export FORGE_SOURCES_DB="${FORGE_SOURCES_DB:-/opt/services/forge/sources.db}"
export MSGVAULT_USER_EMAIL="${MSGVAULT_USER_EMAIL:?MSGVAULT_USER_EMAIL must be set in /opt/services/msgvault/.env}"

mkdir -p /opt/services/msgvault/logs

exec /opt/services/msgvault/bin/msgvault authority recompute \
    --forge-sources-dir "$FORGE_SOURCES_DIR" \
    --forge-sources-db "$FORGE_SOURCES_DB" \
    --user-email "$MSGVAULT_USER_EMAIL"
