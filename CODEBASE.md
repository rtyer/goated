# Codebase & Architecture

## Project structure

```
.                        # Go module root
├── cmd/
│   ├── goated/          # Agent CLI (./workspace/goat)
│   └── goated/          # CLI + daemon (./goated, ./workspace/goat)
├── internal/
│   ├── app/             # Config (env vars, .env loading)
│   ├── agent/           # Provider-neutral runtime contracts
│   ├── claude/          # Claude runtime implementations
│   ├── codex/           # Codex runtime implementations
│   ├── cron/            # Cron runner
│   ├── db/              # BoltDB persistence (crons, subagent runs, meta)
│   ├── gateway/         # Gateway service (message routing, auto-compact, retry)
│   ├── slack/           # Slack connector (Socket Mode)
│   ├── subagent/        # Headless subagent launcher
│   ├── telegram/        # Telegram connector
│   ├── tmux/            # Shared tmux helpers
│   └── util/            # Markdown conversion, etc.
├── workspace/           # Agent working directory (where the active runtime runs)
│   ├── goat             # Agent CLI binary (built by build.sh)
│   ├── GOATED.md        # Shared runtime instructions
│   ├── CLAUDE.md        # Claude compatibility shim
│   ├── TOOLS.md         # Guide for building CLI tools
│   └── self/            # Private agent data (gitignored)
├── build.sh             # Builds all three binaries
├── build_all_and_run_daemon.sh  # Builds + starts daemon
└── main.go              # Alias entrypoint (same as cmd/goated)
```

## Binaries

| Binary | Source | Output path | Purpose |
|--------|--------|-------------|---------|
| `goated` | `.` (main.go) | `./goated` | Control CLI + daemon (`daemon run`, `start`, `cron`, `bootstrap`) |
| `goat` | `./cmd/goated` | `./workspace/goat` | Agent CLI (used by the runtime inside workspace) |

Both are statically-compiled Go. The daemon uses ~14 MB RSS. The `goat` CLI is exec'd per-call and exits immediately.

## How it works

```
┌──────────┐         ┌──────────────┐     paste    ┌──────────────────────────┐
│  Slack/  │ ──────> │   Gateway    │ ───────────> │  Active Runtime (tmux)   │
│ Telegram │         │   Daemon     │              │  interactive session     │
│   User   │ <────── │              │ <──────────  │                          │
└──────────┘         └──────────────┘  exec        └──────────────────────────┘
    ^                    │                           │            │
    │                    │                           │            │ ./goat spawn-subagent
    │                    │         ./goat send_user_ │            │
    │                    │                 message   v            v
    └────────────────────┼────────────────────────────      ┌────────────────────┐
                         │                                  │  Subagent          │
                    ┌────v─────┐                            │ (headless runtime) │
                    │   Cron   │ ────────────────────────>  │                    │
                    │  Runner  │  spawn                     └────────────────────┘
                    └──────────┘
```

**Message flow:**

1. User sends a message via Slack or Telegram
2. Gateway connector receives it (Socket Mode / polling / webhook)
3. Gateway posts a `_thinking..._` indicator (Slack) or typing animation (Telegram)
4. Message is wrapped in a **pydict envelope** (Python dict literal with message, source, chat_id, respond_with, formatting)
5. The selected session runtime pastes the envelope into the tmux pane via `tmux load-buffer` + `paste-buffer`
6. Bridge polls for idle using **content-change detection**: pane must be stable (unchanged across consecutive 2s captures) AND contain `❯`
7. The active runtime processes the request and pipes markdown into `./goat send_user_message --chat <id>`
8. The `goat` CLI converts markdown to platform format (Slack mrkdwn / Telegram HTML) and posts it
9. On Slack, the thinking indicator is deleted; if the runtime is still busy, a new one is posted and reaped on idle

**Key design choice:** the runtime sends its own replies. The gateway doesn't scrape output from tmux — the runtime is instructed to pipe its response through the `goat` CLI.

**Subagents and cron jobs** run as headless runtime processes (not in the tmux session). Claude uses `claude -p`; Codex uses `codex exec`. Each gets its own process, tracked in BoltDB with PID and status.

## Gateway features

- **Auto-compact:** checks context usage every 5 messages using the active runtime's context-estimate capability. If usage exceeds 80% and compaction is supported, sends `/compact` and queues incoming messages until done.
- **Retry on API errors:** detects 5xx/overloaded errors in the pane and retries up to 2 times.
- **Session health:** classifies recoverable vs non-recoverable runtime failures. Auto-restarts recoverable failures up to 5 times. DMs admin if recovery fails.
- **Thinking indicator (Slack):** posts `_thinking..._` on message receipt, deletes it when the runtime responds. TTL reaper (4min soft / 20min hard) prevents orphaned indicators.
- **Idle detection:** runtime-specific. Claude uses stable-pane plus prompt detection; Codex uses pane stability plus blocker classification.

## Cron system

- Cron jobs are stored in BoltDB with schedule, prompt, timezone, and flags.
- The runner ticks every minute, checks due jobs, spawns subagents.
- Jobs with `--silent` flag suppress both user messages and main session notifications on success (errors always notify).
- A job won't fire again if its previous run is still in-flight.

## Configuration

All config via environment variables or `.env` in the repo root:

| Variable | Default | Description |
|----------|---------|-------------|
| `GOAT_GATEWAY` | `telegram` | `slack` or `telegram` |
| `GOAT_AGENT_RUNTIME` | `claude` | `claude` or `codex` |
| `GOAT_SLACK_BOT_TOKEN` | | Bot User OAuth Token (xoxb-...) |
| `GOAT_SLACK_APP_TOKEN` | | App-Level Token (xapp-...) for Socket Mode |
| `GOAT_SLACK_CHANNEL_ID` | | Monitored Slack DM channel |
| `GOAT_TELEGRAM_BOT_TOKEN` | | Telegram bot API token |
| `GOAT_DEFAULT_TIMEZONE` | `America/Los_Angeles` | Timezone for cron schedules |
| `GOAT_ADMIN_CHAT_ID` | | Chat ID for admin alerts |
| `GOAT_DB_PATH` | `./goated.db` | BoltDB path |
| `GOAT_WORKSPACE_DIR` | `workspace` | Agent working directory |
| `GOAT_LOG_DIR` | `./logs` | Log directory |
