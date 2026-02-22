# Remo TUI Enhancement Plan

## Phase 1: Full-Screen TUI Mode

### Change Required:
File: `internal/client/client.go` line 110
Change from:
```go
client.uiProgram = tea.NewProgram(model, tea.WithContext(ctx))
```
To:
```go
client.uiProgram = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
```

### Impact:
- TUI will now use alternate screen buffer (full-screen mode)
- Terminal screen will clear and restore on exit
- Same behavior as vim/htop

## Phase 2: Connections View (TUI)

### Design:
Add `Tab` key to switch between:
- **Logs View** (current): Request logs table
- **Connections View** (new): List of current user's connections

### Connections View Layout:
```
┌─ Connections ──────────────────────────────────────────┐
│ Subdomain │ Status │ Uptime  │ Port   │ [x] Kill     │
│ test      │ ● ON   │ 5m 23s  │ 34291  │ press x      │
│ myapp     │ ● ON   │ 12m 7s  │ 38421  │ press x      │
└────────────────────────────────────────────────────────┘
Navigation: ↑/↓ arrows, x to kill, Tab to switch views
Refresh: Every 5 seconds (configurable)
```

### State Storage:
- File: `~/.remo/state.json`
- Tracks: subdomain, pid, start_time, port, last_ping
- Updated on connect/disconnect/kill

## Phase 3: CLI Commands

### `remo connections`
```
$ remo connections
Subdomain    Status    Uptime    Port    
test         ● ON      5m 23s    34291   
myapp        ● ON      12m 7s    38421   
```

### `remo kill <subdomain>`
- Kills specific connection
- Confirms: "Kill connection to 'test'? (y/n)"
- Sends unregister to server
- Kills SSH process
- Updates state file

### `remo kill --all`
- Kills all current user connections
- Confirmation: "Kill all N connections? (y/n)"

## Phase 4: Error Handling (404 vs 502)

### Server Changes:
Change from 502 to 404 for:
- "no tunnel available" → 404
- "no upstream available" → 404

### Headers:
- `X-Remo-Error: no-tunnel` or `no-upstream`

### Purpose:
- Prevents attackers from enumerating valid subdomains
- Both errors return same 404 status

## Phase 5: Inline Error Display

### TUI Error Banner:
```
┌─ Logs ─────────────────────────────────────────────────┐
│ test | connected | https://test.cloud.remoapps.site    │
├────────────────────────────────────────────────────────┤
│ ⚠ Error: No upstream available (404)                   │
│   Subdomain: test | Time: 14:32:05 | Press any key     │
├────────────────────────────────────────────────────────┤
│ Time     Method  Path       Status  Latency  Remote    │
└────────────────────────────────────────────────────────┘
```

### Error Types:
- **no-upstream**: Local service not responding
- **no-tunnel**: SSH tunnel disconnected
- Both show inline banner with details
- Auto-clear after 30s or press any key

## Implementation Order:
1. Phase 1: Full-screen TUI (immediate)
2. Phase 4: Error handling (1 hour)
3. Phase 2: Connections view (3-4 hours)
4. Phase 5: Inline errors (2 hours)
5. Phase 3: CLI commands (2-3 hours)

## Configuration:
```bash
# TUI refresh interval (default: 5s)
remo connect --refresh-interval 5s

# Or in ~/.remo/config.yaml
refresh_interval: 5s
fullscreen: true
```
