# Plan: Bounded seenEvents map in Slack connector

## Problem

`internal/slack/connector.go` uses `seenEvents map[string]bool` to deduplicate retried Slack
events. This map grows unboundedly for the lifetime of the daemon. Long-running daemons will
leak memory — one entry per event, never pruned.

## Solution

Replace the unbounded map with a time-based eviction strategy. Slack retries events within
a short window (typically <5 minutes). We only need to remember events for ~10 minutes.

Use a simple approach: store `map[string]time.Time` (event ID → first-seen time) and prune
entries older than 10 minutes on each insert.

## Changes

### `internal/slack/connector.go`

1. Change struct field:
```go
seenEvents map[string]time.Time // dedup retried Slack events
```

2. Change constructor:
```go
seenEvents: make(map[string]time.Time),
```

3. Replace the dedup check (currently around line 137-143) with:
```go
func (c *Connector) isDuplicate(eventID string) bool {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()

    // Prune entries older than 10 minutes
    for k, t := range c.seenEvents {
        if now.Sub(t) > 10*time.Minute {
            delete(c.seenEvents, k)
        }
    }

    if _, seen := c.seenEvents[eventID]; seen {
        return true
    }
    c.seenEvents[eventID] = now
    return false
}
```

4. Update the call site in the event handler to use `c.isDuplicate(eventsAPIEvent.EventID)`
   (or whatever key is currently used — verify the exact field name).

## Why not LRU?

An LRU cache would work but adds a dependency or custom implementation for no real benefit.
The time-based approach is simpler, matches Slack's retry semantics, and the prune loop over
a map that's always <1000 entries is negligible cost.

## Risk

- None meaningful. The 10-minute window far exceeds Slack's retry window.
- The prune loop runs on every event, but the map will rarely exceed a few hundred entries.

## Testing

Run the daemon for an extended period. Verify via logs or debug endpoint that the map size
stays bounded. Manually test dedup by sending a message and confirming no double-processing.
