# Plan: Pre-compaction memory flush

## Problem

When auto-compaction triggers (context >80%), the gateway sends `/compact` immediately.
Claude loses whatever's in the context window without being prompted to save important state first.
OpenClaw solves this by injecting a "save anything important" reminder before compaction.

## Solution

Before sending `/compact`, paste a short instruction asking Claude to persist critical context,
then wait for idle before proceeding with `/compact`.

## Changes

### `internal/gateway/service.go` — `compactAndFlush()`

After the "wait for idle" loop (line ~190) and before sending `/compact` (line ~194):

1. Call `s.Bridge.SendRaw(ctx, "Before I compact your context, save any important state to your self/ files now. Be quick — just key facts, decisions, and in-progress work.")`
2. Wait for idle again (same loop pattern as lines 179-190)
3. Then proceed with `/compact` as before

### Rough diff

```go
// --- existing: wait for idle ---

// NEW: ask Claude to flush memory before compaction
fmt.Fprintf(os.Stderr, "[%s] requesting memory flush before compaction\n", time.Now().Format(time.RFC3339))
if err := s.Bridge.SendRaw(ctx, "Before I compact your context, save any important state to your self/ files now. Be quick — just key facts, decisions, and in-progress work."); err != nil {
    fmt.Fprintf(os.Stderr, "[%s] memory flush request failed: %v\n", time.Now().Format(time.RFC3339), err)
    // Continue with compaction anyway
} else {
    time.Sleep(3 * time.Second)
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        busy, err := s.Bridge.IsSessionBusy(ctx)
        if err == nil && !busy {
            break
        }
        time.Sleep(2 * time.Second)
    }
}

// --- existing: send /compact ---
```

## Risk

- Adds ~10-30s to compaction cycle (Claude needs to save files)
- If Claude ignores the instruction or takes too long, the idle-wait loop will timeout naturally
- No behavioral change if Claude has nothing to save

## Testing

Send enough messages to trigger compaction (>80% context). Verify in daemon logs that
"requesting memory flush" appears before "/compact". Check that Claude's self/ files
were updated between the two log lines.
