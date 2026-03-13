# Plan: Fix thinking indicator race condition

## Problem

The thinking indicator timestamp is shared between processes via `/tmp/goated-slack-thinking`:

- **Daemon process** (`slack/connector.go`): `postThinking()` writes, `clearThinkingIfNeeded()` reads/deletes, `reapThinkingIndicator()` goroutine reads/deletes
- **CLI process** (`send_user_message.go`): reads/deletes the file, may post a new indicator

Multiple goroutines and separate processes race on this file with no synchronization.
This can cause orphaned thinking messages in Slack or double-deletion errors.

## Solution

Use atomic file operations to eliminate the race:

1. **Write**: Use `os.WriteFile` to a temp file + `os.Rename` (atomic on Linux)
2. **Read-and-delete**: Use `os.Rename` to a process-local temp path first (atomic claim), then read. If rename fails, another process already claimed it.

This gives us cross-process mutual exclusion without a lock file or IPC.

## Changes

### New helper: `internal/slack/thinking.go`

```go
package slack

import (
    "os"
    "path/filepath"
)

const ThinkingFile = "/tmp/goated-slack-thinking"

// WriteThinkingTS atomically writes the thinking indicator timestamp.
func WriteThinkingTS(ts string) error {
    tmp := ThinkingFile + ".tmp"
    if err := os.WriteFile(tmp, []byte(ts), 0644); err != nil {
        return err
    }
    return os.Rename(tmp, ThinkingFile)
}

// ClaimThinkingTS atomically reads and removes the thinking indicator timestamp.
// Returns "" if no indicator exists or another process already claimed it.
func ClaimThinkingTS() string {
    // Rename to a unique temp name to atomically claim ownership
    claimed := ThinkingFile + ".claimed." + fmt.Sprintf("%d", os.Getpid())
    if err := os.Rename(ThinkingFile, claimed); err != nil {
        return "" // file doesn't exist or already claimed
    }
    data, err := os.ReadFile(claimed)
    os.Remove(claimed)
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(data))
}
```

### Update call sites

**`slack/connector.go::postThinking()`** — replace `os.WriteFile(ThinkingFile, ...)` with `WriteThinkingTS(ts)`

**`slack/connector.go::clearThinkingIfNeeded()`** — use `ClaimThinkingTS()` instead of `os.Remove`

**`slack/connector.go::reapThinkingIndicator()`** — use `ClaimThinkingTS()` to check if file still exists

**`cmd/goated/cli/send_user_message.go`** — replace `os.ReadFile` + `os.Remove` with `ClaimThinkingTS()`

### Move ThinkingFile constant

Currently `ThinkingFile` is exported from `slack/connector.go`. Move it to the new `thinking.go`
file and update imports in `send_user_message.go`.

## Risk

- `os.Rename` is atomic on Linux (same filesystem), which `/tmp` satisfies
- If the daemon and CLI are on different filesystems (unlikely for `/tmp`), rename could fail — but that's the current behavior too
- The `.claimed.<pid>` suffix prevents collisions between concurrent CLI invocations

## Testing

1. Send a message, verify thinking indicator appears and is cleaned up on response
2. Send rapid-fire messages — verify no orphaned indicators
3. Kill the daemon mid-response — verify the TTL reaper still cleans up
