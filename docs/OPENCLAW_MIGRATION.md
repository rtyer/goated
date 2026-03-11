# Migrating from OpenClaw to Goated

OpenClaw stores credentials in environment variables and uses its own cron system. Goated uses file-backed credentials (`workspace/creds/*.txt`) and a built-in cron runner with bbolt storage.

## Credentials

OpenClaw typically injects secrets via environment variables. In goated, the agent manages its own credentials with `./goat creds`.

### Migrating secrets

For each secret your agent uses, store it via the CLI:

```sh
./goat creds set GITHUB_API_KEY ghp_xxxxxxxxxxxx
./goat creds set OPENAI_API_KEY sk-xxxxxxxxxxxx
./goat creds set SMTP_PASSWORD mypassword
```

Or tell the agent to do it in a Telegram message:

> Store my GitHub token as a credential: ghp_xxxxxxxxxxxx

The agent will run `./goat creds set GITHUB_API_KEY ghp_xxxxxxxxxxxx` and confirm.

### Reading secrets in agent prompts

Tell the agent to read credentials at runtime. Example CLAUDE.md snippet:

```
When you need an API key, read it with: ./goat creds get <KEY_NAME>
Available credentials: ./goat creds list
```

The agent reads creds from `workspace/creds/<KEY_NAME>.txt` — no environment variables needed.

## Cron jobs

OpenClaw uses external cron (system crontab or similar). Goated has a built-in cron runner that fires subagents with bbolt-backed dedup.

### Migrating cron jobs

If you had a system cron like:

```
0 9 * * * cd /path/to/openclaw && claude -p "Check my email and summarize"
```

Replace it with:

```sh
./goated cron add \
  --chat <your_chat_id> \
  --schedule "0 9 * * *" \
  --prompt "Check my email and summarize the important ones"
```

Or for complex prompts, use a file:

```sh
./goated cron add \
  --chat <your_chat_id> \
  --schedule "0 9 * * *" \
  --prompt-file /path/to/workspace/prompts/morning-email.md
```

The prompt file is read at execution time, so you can edit it without re-registering the cron.

### Example prompt files

**Morning email digest** (`prompts/morning-email.md`):
```
Check my email inbox for unread messages from the last 24 hours.
Summarize the important ones and flag anything that needs a reply today.
Skip newsletters and marketing emails.
```

**Daily PR review** (`prompts/pr-review.md`):
```
Check open pull requests on our main repos.
For each PR, give a one-line summary and flag any that have been open more than 3 days.
If any PRs are approved but not merged, mention those too.
```

**Weekly project status** (`prompts/weekly-status.md`):
```
Generate a weekly status report covering:
- Commits merged this week across all repos
- Open issues created vs closed
- Any blockers or stale PRs
Format it as a brief summary I can forward to the team.
```

### Telling the agent to set up its own crons

You can ask the agent directly via Telegram:

> Set up a cron job that checks my email every morning at 9am Pacific and sends me a summary.

The agent will run:
```sh
./goat cron add --chat <chat_id> --schedule "0 9 * * *" --prompt "Check email and summarize"
```

### Managing cron jobs

```sh
# List all cron jobs
./goat cron list

# Disable without deleting
./goat cron disable <id>

# Re-enable
./goat cron enable <id>

# Delete permanently
./goat cron remove <id>
```

## Key differences from OpenClaw

| Feature | OpenClaw | Goated |
|---------|----------|--------|
| Credentials | Environment variables | File-backed (`creds/*.txt`) via `./goat creds` |
| Cron | System crontab | Built-in runner with bbolt, dedup, 1hr timeout |
| Agent messaging | Delimiter-based capture | `./goat send_user_message --chat <id>` |
| Session management | Manual | Auto-healing with health checks and restart |
| Subagents | N/A | `./goat spawn-subagent --prompt "..."` |
| Daemon | N/A | `./goated daemon restart --reason "..."` |
