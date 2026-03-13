# Plan: Wire up SendAndWait timeout parameter

## Problem

`TmuxBridge.SendAndWait()` accepts a timeout parameter but ignores it (`_ time.Duration`).
Callers pass `30*time.Minute` expecting the method to respect it. Currently the method just
pastes the envelope and returns immediately — it doesn't wait at all, despite its name.

The gateway's `sendWithRetry()` then calls `tmux.WaitForIdle()` separately with a hardcoded
`postSendTimeout` of 5 minutes. The 30-minute timeout from the caller is lost.

## Solution

Make `SendAndWait` actually wait for Claude to finish processing, respecting the caller's
timeout. This consolidates the wait logic into the bridge where it belongs.

## Changes

### `internal/claude/tmux_bridge.go` — `SendAndWait()`

```go
func (b *TmuxBridge) SendAndWait(ctx context.Context, channel, chatID string, userPrompt string, timeout time.Duration) error {
    if err := b.EnsureSession(ctx); err != nil {
        return err
    }

    wrapped := buildPromptEnvelope(channel, chatID, userPrompt)
    if err := tmux.PasteAndEnter(ctx, wrapped); err != nil {
        return err
    }

    // Wait for Claude to process and return to prompt
    return tmux.WaitForIdle(ctx, timeout)
}
```

### `internal/gateway/service.go` — `sendWithRetry()`

Remove the separate `tmux.WaitForIdle()` call after `SendAndWait()`, since `SendAndWait`
now handles the wait internally. The error-checking logic (scanning pane for API errors
after idle) should remain.

Current flow in `sendWithRetry()`:
1. `Bridge.SendAndWait(...)` — paste only (currently)
2. Separate idle wait with `postSendTimeout`
3. Check pane for errors

New flow:
1. `Bridge.SendAndWait(...)` — paste + wait (timeout from caller)
2. Check pane for errors

### Update `postSendTimeout`

The constant `postSendTimeout = 5 * time.Minute` in service.go can be removed or kept as
a default. The actual timeout is now passed through from the caller (currently 30 min for
normal messages, 30 min for compaction flush).

### `waitForIdleOrStall` in tmux_bridge.go

This method (used elsewhere) already implements wait logic. Consider whether `SendAndWait`
should use `waitForIdleOrStall` instead of `WaitForIdle` for stall detection. Likely yes —
if Claude stalls for 30s with no output and no prompt, we should return rather than wait
the full timeout.

## Risk

- Callers that currently rely on the fire-and-forget behavior of `SendAndWait` will now
  block until Claude finishes. This is the correct behavior — verify no caller expects
  immediate return.
- The 30-minute timeout is generous. If Claude hangs, the gateway blocks for 30 min before
  retrying. This matches the current `sendWithRetry` behavior (5 min idle wait + retries).

## Testing

Send a message through the gateway. Verify in logs that `SendAndWait` blocks until Claude
responds (not just until paste completes). Verify that timeout is respected by sending a
very long-running task and confirming it doesn't block forever.
