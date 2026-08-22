# msgvault

[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/Docs-msgvault.io-blue)](https://msgvault.io)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/fDnmxB8Wkq)

[Documentation](https://msgvault.io) · [Setup Guide](https://msgvault.io/guides/oauth-setup/) · [Interactive TUI](https://msgvault.io/usage/tui/)

> **Alpha software.** APIs, storage format, and CLI flags may change without notice. Back up your data.

Archive a lifetime of email. Analytics and search in milliseconds, entirely offline.

## Why msgvault?

Your messages are yours. Decades of correspondence, attachments, and history shouldn't be locked behind a web interface or an API. msgvault downloads a complete local copy and then everything runs offline. Search, analytics, and the MCP server all work against local data with no network access required.

Currently supports Gmail and IMAP sync, plus offline imports from MBOX exports and Apple Mail (.emlx) directories.

## Features

- **Full Gmail backup**: raw MIME, attachments, labels, and metadata
- **IMAP sync**: archive mail from any standard IMAP server
- **MBOX / Apple Mail import**: import email from MBOX exports or Apple Mail (.emlx) directories
- **Interactive TUI**: drill-down analytics over your entire message history, powered by DuckDB over Parquet — connects to a remote `msgvault serve` instance or runs locally
- **Web UI**: React SPA in `web/` — Dashboard, Messages, Aggregate, Search, Deletions, Thread, and Message detail views, served by the Go binary via embed
- **Full-text search**: FTS5 with Gmail-like query syntax (`from:`, `has:attachment`, date ranges)
- **MCP server**: access your full archive at the speed of thought in Claude Desktop and other MCP-capable AI agents
- **DuckDB analytics**: millisecond aggregate queries across hundreds of thousands of messages in the TUI, CLI, and MCP server
- **Incremental sync**: History API picks up only new and changed messages
- **Multi-account**: archive several Gmail and IMAP accounts in a single database
- **Resumable**: interrupted syncs resume from the last checkpoint
- **Content-addressed attachments**: deduplicated by SHA-256

## Installation

**macOS / Linux:**
```bash
curl -fsSL https://msgvault.io/install.sh | bash
```

**Windows (PowerShell):**
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://msgvault.io/install.ps1 | iex"
```

The installer detects your OS and architecture, downloads the latest release from [GitHub Releases](https://github.com/wesm/msgvault/releases), verifies the SHA-256 checksum, and installs the binary. You can review the script ([bash](https://msgvault.io/install.sh), [PowerShell](https://msgvault.io/install.ps1)) before running, or download a release binary directly from GitHub.

To build from source instead (requires **Go 1.25+** and a C/C++ compiler for CGO and to statically link DuckDB):

```bash
git clone https://github.com/wesm/msgvault.git
cd msgvault
make install
```

**Conda-Forge:**

You can install msgvault [from conda-forge](https://prefix.dev/channels/conda-forge/packages/msgvault) using Pixi or Conda:

```bash
pixi global install msgvault
conda install -c conda-forge msgvault
```

## Quick Start

> **Prerequisites:** You need a Google Cloud OAuth credential before adding an account.
> Follow the **[OAuth Setup Guide](https://msgvault.io/guides/oauth-setup/)** to create one (~5 minutes).

```bash
msgvault init-db
msgvault add-account you@gmail.com          # opens browser for OAuth
msgvault sync-full you@gmail.com --limit 100
msgvault tui
```

## Commands

| Command | Description |
|---------|-------------|
| `init-db` | Create the database |
| `add-account EMAIL` | Authorize a Gmail account (use `--headless` for servers) or add an IMAP account |
| `sync-full EMAIL` | Full sync (`--limit N`, `--after`/`--before` for date ranges) |
| `sync EMAIL` | Sync only new/changed messages |
| `tui` | Launch the interactive TUI (`--account` to filter, `--local` to force local) |
| `search QUERY` | Search messages (`--account` to filter, `--json` for machine output) |
| `show-message ID` | View full message details (`--json` for machine output) |
| `mcp` | Start the MCP server for AI assistant integration |
| `serve` | Run daemon with scheduled sync and HTTP API for remote TUI |
| `stats` | Show archive statistics |
| `list-accounts` | List synced email accounts |
| `verify EMAIL` | Verify archive integrity against Gmail |
| `export-eml` | Export a message as `.eml` |
| `import-mbox` | Import email from an MBOX export or `.zip` of MBOX files |
| `import-emlx` | Import email from an Apple Mail directory tree |
| `build-cache` | Rebuild the Parquet analytics cache |
| `update` | Update msgvault to the latest version |
| `setup` | Interactive first-run configuration wizard |
| `repair-encoding` | Fix UTF-8 encoding issues |
| `list-senders` / `list-domains` / `list-labels` | Explore metadata |

See the [CLI Reference](https://msgvault.io/cli-reference/) for full details.

## Triage pipeline (cross-repo with forge)

Score archived Gmail against the [forge](https://github.com/quartermint/forge) (local checkout `~/lattice`) knowledge graph and surface a weekly digest of high-signal candidates for forge ingestion. Pure SQL + regex + lexical scoring — **no LLM/encoder calls in the hot path**.

### `msgvault triage`

Reads forge `graph.db` and `sources.db` read-only, scores each message in the lookback window via a 7-criterion composite (vocab 0.25, url_gold 0.20, curiosity 0.15, recurrence 0.15, bridge 0.10, decision 0.10, expert 0.05), applies hard filters (List-Unsubscribe, calendar invites, receipts, 2FA, large bcc threads, short messages without URL signal), and emits the top-N candidates above threshold ≥0.55 as JSONL.

```bash
msgvault triage \
    --since 7d \
    --out /tmp/triage.jsonl \
    --forge-graph /opt/services/forge/graph.db \
    --forge-sources /opt/services/forge/sources.db \
    --user-email ryan@example.com
```

Other flags: `--noise-domains` (TOML override), `--max` (default 25), `--threshold`
(default 0.55), `--limit` (default 5000 messages scored per run).

Output is byte-identical for byte-identical inputs (deterministic sort: `score DESC, date DESC, message_id ASC`).

### `msgvault authority recompute`

Per-sender authority scoring in `[0,1]`, used by triage's expert criterion.

```bash
msgvault authority recompute            # incremental, from the stored watermark
msgvault authority recompute --full     # rebuild from scratch
```

⛔ **`trusted_contacts.toml` and `msgvault trusted-contacts bootstrap` no longer exist.**
The `trustedcontacts` package and its cobra command were deleted (AUTHGRAPH-03); triage's
expert scoring now reads the authority store instead. The `--trusted-contacts` flag is gone
— passing it is an error, not a no-op. Run `authority recompute` instead; a launchd plist
and `run-authority.sh` are provided for a daily recompute.

### `msgvault digest send`

Send the weekly markdown digest email via the existing Gmail OAuth.

**One-time setup:** the `gmail.send` scope is NEW. Existing tokens have only `gmail.readonly + gmail.modify`, so you must re-grant interactively before enabling the launchd cron:

```bash
msgvault add-account --reauth --scopes=triage <your-email>
```

Then send (use `--dry-run` to print the email to stdout without hitting Gmail):

```bash
msgvault digest send \
    --in /tmp/triage.jsonl \
    --to ryan@example.com \
    --from ryan@example.com \
    --account ryan@example.com
```

Each digest row is numbered 1..N matching the JSONL row index, so the recipient can scan the email, note row numbers, and approve via `forge ingest --from-triage <jsonl> --select 1,3,7` on the forge side.

### Weekly cron

⚠ The launchd plists below were written for the Mac Mini, which was wiped 2026-07-09 and
rebuilt as `hammer` (`100.98.187.0`). msgvault's iMessage sync runs there now; check where
these timers are actually installed before assuming they fire.


A launchd plist at `launchd/com.msgvault.triage-digest.plist` runs the full `triage` → `digest send` pipeline every Monday 07:00 PT:

```bash
sudo cp launchd/com.msgvault.triage-digest.plist /Library/LaunchDaemons/
cp launchd/run-triage.sh /opt/services/msgvault/run-triage.sh
chmod +x /opt/services/msgvault/run-triage.sh
launchctl bootstrap gui/$(id -u) /Library/LaunchDaemons/com.msgvault.triage-digest.plist
```

Required env in `/opt/services/msgvault/.env`:

```
DIGEST_TO_ADDR=ryan@example.com
DIGEST_FROM_ADDR=ryan@example.com
MSGVAULT_ACCOUNT=ryan@example.com
```

The launchd plist deliberately omits `ProcessType` — setting it to `Background` causes macOS to suspend the service unpredictably (see Mac Mini operational notes).

## Vector Search

msgvault can search your archive semantically using vector embeddings in addition to the default FTS5 keyword search. Point it at a self-hosted OpenAI-compatible embedding endpoint (Ollama, llama.cpp, LM Studio) and three surfaces accept either pure semantic search or BM25+vector fused via Reciprocal Rank Fusion:

- **CLI:** `msgvault search "..." --mode vector` or `--mode hybrid`
- **HTTP:** `GET /api/v1/search?q=...&mode=vector` or `mode=hybrid`
- **MCP:** the `search_messages` tool with a `mode` argument set to `vector` or `hybrid`

A separate MCP tool, `find_similar_messages`, returns nearest neighbors for a seed message. See the [Vector Search guide](https://msgvault.io/usage/vector-search/) for setup, backfill, and troubleshooting.

## Importing from MBOX or Apple Mail

Import email from providers that offer MBOX exports or from a local Apple Mail data directory:

```bash
msgvault init-db
msgvault import-mbox you@example.com /path/to/export.mbox
msgvault import-mbox you@example.com /path/to/export.zip   # zip of MBOX files
msgvault import-emlx                                        # auto-discover Apple Mail accounts
msgvault import-emlx you@example.com ~/Library/Mail/V10     # explicit path
```

## Configuration

All data lives in `~/.msgvault/` by default (override with `MSGVAULT_HOME`).

```toml
# ~/.msgvault/config.toml
[oauth]
client_secrets = "/path/to/client_secret.json"

[sync]
rate_limit_qps = 5
```

See the [Configuration Guide](https://msgvault.io/configuration/) for all options.

### Multiple OAuth Apps (Google Workspace)

Some Google Workspace organizations require OAuth apps within their org.
To use multiple OAuth apps, add named apps to `config.toml`:

```toml
[oauth]
client_secrets = "/path/to/default_secret.json"   # for personal Gmail

[oauth.apps.acme]
client_secrets = "/path/to/acme_workspace_secret.json"
```

Then specify the app when adding accounts:

```bash
msgvault add-account you@acme.com --oauth-app acme
msgvault add-account personal@gmail.com              # uses default
```

To switch an existing account to a different OAuth app:

```bash
msgvault add-account you@acme.com --oauth-app acme   # re-authorizes
```

## MCP Server

msgvault includes an MCP server that lets AI assistants search, analyze, and read your archived messages. Connect it to Claude Desktop or any MCP-capable agent and query your full message history conversationally. See the [MCP documentation](https://msgvault.io/usage/chat/) for setup instructions.

## Daemon Mode (NAS/Server)

Run msgvault as a long-running daemon for scheduled syncs and remote access:

```bash
msgvault serve
```

Configure scheduled syncs in `config.toml`:

```toml
[[accounts]]
email = "you@gmail.com"
schedule = "0 2 * * *"   # 2am daily (cron)
enabled = true

[server]
api_port = 8080
bind_addr = "0.0.0.0"
api_key = "your-secret-key"
```

The TUI can connect to a remote server by configuring `[remote].url`. Use `--local` to force local database when remote is configured. See the [Web Server reference](https://msgvault.io/api-server/) for the HTTP API.

## Documentation

- [Setup Guide](https://msgvault.io/guides/oauth-setup/): OAuth, first sync, headless servers
- [Searching](https://msgvault.io/usage/searching/): query syntax and operators
- [Interactive TUI](https://msgvault.io/usage/tui/): keybindings, views, deletion staging
- [CLI Reference](https://msgvault.io/cli-reference/): all commands and flags
- [Multi-Account](https://msgvault.io/usage/multi-account/): managing multiple Gmail accounts
- [Configuration](https://msgvault.io/configuration/): config file and environment variables
- [Architecture](https://msgvault.io/architecture/storage/): SQLite, Parquet, and attachment storage
- [MCP Server](https://msgvault.io/usage/chat/): AI assistant integration
- [Troubleshooting](https://msgvault.io/troubleshooting/): common issues and fixes
- [Development](https://msgvault.io/development/): contributing, testing, building

## Community

Join the [msgvault Discord](https://discord.gg/fDnmxB8Wkq) to ask questions, share feedback, report issues, and connect with other users.

## Development

```bash
git clone https://github.com/wesm/msgvault.git
cd msgvault
make install-hooks  # install pre-commit hook (requires prek)
make test           # run tests
make lint           # run linter (auto-fix)
make install        # build and install
```

Pre-commit hooks are managed by [prek](https://prek.j178.dev/) (`brew install prek`).

## License

MIT. See [LICENSE](LICENSE) for details.
