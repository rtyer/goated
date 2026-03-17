# Ideas from Hermes-Agent for Goated

Source: https://github.com/NousResearch/hermes-agent

## High Impact, Worth Doing Soon

### 1. Structured Memory with CRUD + Injection Scanning

Hermes has `memory_tool.py` with bounded memory stores (`MEMORY.md`, `USER.md`), add/replace/remove operations, and an injection scanner that blocks prompt injection patterns before they get persisted. Memory is loaded as a frozen snapshot at session start and never mutated mid-session (preserving prompt cache).

**For goated:** A `goat memory add/replace/remove` command would let agents curate persistent knowledge across sessions, with size bounds to prevent context bloat. The injection scanner is particularly relevant since goated injects envelope data into prompts -- the same threat patterns apply.

### 2. Cross-Session Search (FTS5)

Hermes indexes past conversation transcripts with SQLite FTS5, then uses a *cheap auxiliary model* (Gemini Flash) to summarize matching results rather than stuffing raw transcripts into context. Resolves child sessions to parent sessions so delegation results are properly attributed.

**For goated:** When the user says "remember when we..." the agent could recall weeks-old context. Adding SQLite FTS5 alongside BoltDB (or migrating to SQLite) would enable this. The auxiliary-model summarization pattern keeps the main agent's context clean.

### 3. Filesystem Checkpoints via Shadow Git

Before any file-mutating operation, Hermes snapshots using a hidden git repo (`GIT_DIR`/`GIT_WORK_TREE` to avoid polluting the user's project). Supports listing, diffing, and rollback (including single-file restore). Takes a "pre-rollback snapshot" before restoring so you can undo the undo. Uses deterministic shadow repo paths via `sha256(abs_dir)[:16]`.

**For goated:** Especially valuable for unsupervised cron jobs and subagents. A `goat rollback` command would be a great safety net. The shadow-git approach requires no additional dependencies beyond git.

### 4. Skills System with Progressive Disclosure

Instead of cramming everything into CLAUDE.md, Hermes stores reusable procedural knowledge in `~/.hermes/skills/` as directories with `SKILL.md` files, optional references, templates, and scripts. Uses YAML frontmatter for metadata. Key insight: listing skills only returns names and descriptions (cheap), viewing a skill loads the full content (expensive). Skills can be per-platform enabled/disabled with prerequisite checks.

**For goated:** A `workspace/skills/` directory with `goat skill list/view <name>` would let the agent load specialized instructions on demand rather than via a monolithic instruction file. Reduces context bloat.

### 5. Session Reset Policies

Hermes implements configurable session reset policies per platform/chat type: `idle` (reset after N minutes of inactivity), `daily` (reset at a specific hour), `both`, or `none`. Background processes prevent auto-reset.

**For goated:** Low effort, prevents stale context accumulation. E.g., auto-fresh session at midnight local time, or after 4 hours of inactivity.

## Medium Impact, Interesting Patterns

### 6. Subagent Shared Budgets + Progress Callbacks

Hermes spawns child agents with *shared iteration budgets* with the parent, restricted toolsets (no recursive delegation, no user interaction, no memory writes), and progress callbacks relayed to the user's display. Batch mode runs up to 3 subagents in parallel via `ThreadPoolExecutor`. Delegation config supports routing subagents to different providers/models. Depth limit (`MAX_DEPTH = 2`) prevents runaway recursion.

**For goated:** Could add: (1) shared iteration/cost budgets so subagents don't run away, (2) progress callbacks so the parent session can show subagent activity in real-time, (3) batch parallel mode, (4) routing subagents to cheaper models (e.g. Haiku for research, Opus for main session).

### 7. Smart Command Approval

Hermes detects dangerous patterns (recursive delete, fork bombs, SQL DROP, etc.), then offers session-scoped or permanent allowlisting. The `smart` approval mode uses an auxiliary LLM to assess whether a flagged command is actually dangerous (reducing false positives). Combines pattern matching with LLM judgment.

**For goated:** A `goat approve/deny` command could let users respond to pending approvals via Slack/Telegram. The smart approval pattern (cheap model to triage) is useful when the user isn't immediately available.

### 8. Context Compressor with Protected Head/Tail

Hermes protects the first N and last N turns while summarizing everything in between using a cheap auxiliary model. Tracks actual token counts from API responses (not estimates). Uses a prefix summary marker (`[CONTEXT COMPACTION]`) that tells the agent "this work was already done, don't repeat it."

**For goated:** Currently goated delegates compaction entirely to the runtime via `/compact`. Hermes's approach gives the orchestrator more control -- choosing what to protect, using a cheap model for summarization, and preventing the agent from repeating already-compacted work.

### 9. Background Process Notifications

When a terminal command runs in the background, Hermes pushes status updates to the user's chat. Verbosity is configurable: `all` (running updates + final), `result` (only final), `error` (only on failure), `off`.

**For goated:** For long-running subagent tasks or cron jobs, push progress updates to Slack/Telegram. Currently the user has to wait for the final result.

### 10. Atomic Writes for All Persistent State

Throughout the codebase, Hermes uses atomic writes (temp file + `os.replace()`) for all persistent state. Go equivalent: write to temp file in same directory, `fsync`, then `os.Rename` (atomic on same filesystem).

**For goated:** BoltDB handles atomicity at the database level, but file-based state (session ID files, hook state, config) would benefit from this pattern.

## Nice to Have

### 11. PII Redaction in Session Context

Hermes hashes user/chat IDs before injecting them into the system prompt, and strips phone numbers on platforms where IDs are phone numbers (WhatsApp, Signal).

**For goated:** Good privacy practice since prompt envelopes may end up in API provider logs. Low effort.

### 12. User Modeling via Dialectic Queries

Hermes integrates with Honcho's AI-native memory system, building a deepening model of who the user is across sessions -- preferences, communication style, workflow patterns. After each session, extracts key facts with a cheap model.

**For goated:** A simpler version: after each session ends, use a cheap model to extract key facts, append to a structured profile file, inject into future system prompts. The async prefetch pattern (prepare context while user is typing) is also applicable to goated's gateway.

### 13. Tool Registry Pattern

Hermes uses a singleton `ToolRegistry` where each tool registers its schema, handler, availability check, and metadata at import time. Supports centralized discovery, schema collection, dispatch, error wrapping, and per-platform enable/disable.

**For goated:** A registry pattern could formalize tool discovery, making it easier to expose available commands dynamically in the system prompt, add new tools without boilerplate, and conditionally enable tools based on environment.
