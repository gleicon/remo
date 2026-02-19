---
phase: 02-tui-request-logging
plan: 02
type: execute
wave: 2
depends_on:
  - 02-tui-request-logging-01
files_modified:
  - internal/client/client.go
autonomous: true
requirements:
  - TUI-01
  - TUI-02

must_haves:
  truths:
    - Client polls server /events endpoint every second
    - Client forwards request events to TUI via RequestLogMsg
    - TUI displays requests in real-time as they arrive
    - Auto-reconnect with exponential backoff on poll failure
  artifacts:
    - path: internal/client/client.go
      provides: Event polling and TUI forwarding
      contains: "pollEvents", "sendUI.*RequestLogMsg"
  key_links:
    - from: internal/client/client.go pollEvents
      to: internal/server/server.go /events endpoint
      via: HTTP GET through SSH tunnel
      pattern: "http.Get.*events"
    - from: internal/client/client.go
      to: internal/tui/model.go RequestLogMsg
      via: sendUI function
      pattern: "tui.RequestLogMsg"
---

<objective>
Implement client-side event polling to fetch request logs from server and forward to TUI.

Purpose: Bridge the server event system with the TUI by polling the /events endpoint through the SSH tunnel and converting events to TUI messages.
Output: Client continuously polls for events and TUI displays them in real-time.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/client/client.go
@internal/server/server.go

Server now has /events endpoint (Plan 01) that returns request events. Client needs to:
1. Poll this endpoint periodically (every 1 second per locked decision)
2. Convert server events to tui.RequestLogMsg
3. Forward to TUI via existing sendUI() function
4. Handle reconnection with exponential backoff on failures

The TUI model already has RequestLogMsg struct and handles it in Update(). Client already has sendUI() helper that sends tea.Msg to the TUI program.
</context>

<tasks>

<task type="auto">
  <name>Add event polling infrastructure to Client</name>
  <files>internal/client/client.go</files>
  <action>
Add the following to Client struct and create event polling infrastructure:

1. Add to Client struct:
   ```go
   eventsClient   *http.Client
   pollInterval   time.Duration
   lastEventIndex int // Track which events we've already sent
   ```

2. In New() function, initialize:
   ```go
   eventsClient: &http.Client{
       Timeout: 5 * time.Second,
   },
   pollInterval: time.Second, // Poll every second per locked decision
   ```

3. Create pollEvents method that runs in a goroutine:
   ```go
   func (c *Client) startEventPolling(ctx context.Context) {
       go func() {
           ticker := time.NewTicker(c.pollInterval)
           defer ticker.Stop()
           
           backoff := c.reconnectMin
           for {
               select {
               case <-ctx.Done():
                   return
               case <-ticker.C:
                   if err := c.pollAndForwardEvents(); err != nil {
                       c.log.Warn().Err(err).Msg("event poll failed, backing off")
                       time.Sleep(backoff)
                       backoff *= 2
                       if backoff > c.reconnectMax {
                           backoff = c.reconnectMax
                       }
                   } else {
                       backoff = c.reconnectMin // Reset on success
                   }
               }
           }
       }()
   }
   ```
  </action>
  <verify>grep -n "startEventPolling\|pollInterval\|eventsClient" internal/client/client.go</verify>
  <done>Client has event polling infrastructure with configurable interval and backoff</done>
</task>

<task type="auto">
  <name>Implement pollAndForwardEvents to fetch and forward to TUI</name>
  <files>internal/client/client.go</files>
  <action>
Create pollAndForwardEvents method that fetches events from server and forwards new ones to TUI:

```go
func (c *Client) pollAndForwardEvents() error {
    // Fetch events from server through the tunnel
    resp, err := c.eventsClient.Get("http://127.0.0.1:18080/events")
    if err != nil {
        return fmt.Errorf("fetch events: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("events endpoint returned %d", resp.StatusCode)
    }
    
    var events []struct {
        Time     time.Time     `json:"time"`
        Method   string        `json:"method"`
        Path     string        `json:"path"`
        Status   int           `json:"status"`
        Latency  time.Duration `json:"latency"`
        Remote   string        `json:"remote"`
        BytesIn  int           `json:"bytes_in"`
        BytesOut int           `json:"bytes_out"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
        return fmt.Errorf("decode events: %w", err)
    }
    
    // Forward new events to TUI
    for i := c.lastEventIndex; i < len(events); i++ {
        evt := events[i]
        c.sendUI(tui.RequestLogMsg{
            Time:     evt.Time,
            Method:   evt.Method,
            Path:     evt.Path,
            Status:   evt.Status,
            Latency:  evt.Latency,
            Remote:   evt.Remote,
            BytesIn:  evt.BytesIn,
            BytesOut: evt.BytesOut,
        })
    }
    
    c.lastEventIndex = len(events)
    return nil
}
```

Add the necessary imports for time and json packages if not already present.
  </action>
  <verify>grep -n "pollAndForwardEvents" internal/client/client.go</verify>
  <done>pollAndForwardEvents method fetches events and forwards to TUI as RequestLogMsg</done>
</task>

<task type="auto">
  <name>Start event polling after successful registration</name>
  <files>internal/client/client.go</files>
  <action>
Modify the runSession method to start event polling after successful registration:

1. After successful registration in runSession (around line 248), add:
   ```go
   // Start polling for request events
   c.startEventPolling(ctx)
   ```

This ensures event polling only starts after the tunnel is established and registered.

2. The polling goroutine will automatically stop when ctx is cancelled (on disconnect or shutdown).
  </action>
  <verify>grep -A2 -B2 "startEventPolling" internal/client/client.go</verify>
  <done>Event polling starts after successful tunnel registration</done>
</task>

</tasks>

<verification>
- [ ] Client polls /events endpoint every second
- [ ] New events are forwarded to TUI as RequestLogMsg
- [ ] Events include all fields from server (time, method, path, status, latency, remote, bytes)
- [ ] Exponential backoff on poll failures
- [ ] Polling stops when context is cancelled
</verification>

<success_criteria>
Client successfully bridges server events to TUI:
1. Polls /events endpoint every second through SSH tunnel
2. Forwards new events to TUI as RequestLogMsg
3. Handles failures with exponential backoff
4. TUI displays incoming requests in real-time
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-02-SUMMARY.md`
</output>
