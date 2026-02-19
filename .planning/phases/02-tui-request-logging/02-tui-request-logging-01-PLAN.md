---
phase: 02-tui-request-logging
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/server/server.go
autonomous: true
requirements:
  - SRV-03

must_haves:
  truths:
    - Server exposes /events endpoint for request log streaming
    - Server captures request details after each proxy request
    - Server maintains circular buffer of last 100 request events
    - Events include timestamp, method, path, status, latency, remote IP, bytes in/out
  artifacts:
    - path: internal/server/server.go
      provides: Request event capture and HTTP endpoint
      contains: "type RequestEvent", "handleEvents", event emission in handleProxy
  key_links:
    - from: internal/server/server.go handleProxy
      to: internal/server/server.go requestEvents buffer
      via: Event capture after proxy.ServeHTTP
      pattern: "s.recordRequest.*RequestEvent"
---

<objective>
Add request event tracking to the server with an HTTP endpoint for TUI polling.

Purpose: Enable real-time request logging by capturing request details after each proxied request and exposing them via a pollable endpoint through the SSH tunnel.
Output: Server emits structured request events accessible via GET /events endpoint.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/server/server.go

The server currently proxies requests via handleProxy() but does not capture or expose request event details. The TUI model already defines RequestLogMsg struct with fields: Time, Method, Path, Status, Latency, Remote, BytesIn, BytesOut. Server needs to capture these same fields and expose them via HTTP polling endpoint.
</context>

<tasks>

<task type="auto">
  <name>Add RequestEvent type and circular buffer to Server</name>
  <files>internal/server/server.go</files>
  <action>
Add the following to the Server struct and create supporting types:

1. Create RequestEvent struct (similar to tui.RequestLogMsg):
   ```go
   type RequestEvent struct {
       Time     time.Time     `json:"time"`
       Method   string        `json:"method"`
       Path     string        `json:"path"`
       Status   int           `json:"status"`
       Latency  time.Duration `json:"latency"`
       Remote   string        `json:"remote"`
       BytesIn  int           `json:"bytes_in"`
       BytesOut int           `json:"bytes_out"`
   }
   ```

2. Add to Server struct:
   ```go
   requestEvents []RequestEvent
   eventsMu      sync.RWMutex
   maxEvents     int // default 100
   ```

3. In New() function, initialize:
   - requestEvents: make([]RequestEvent, 0, 100)
   - maxEvents: 100

Use sync.RWMutex for thread-safe access to the events slice.
  </action>
  <verify>grep -n "RequestEvent" internal/server/server.go | head -5</verify>
  <done>RequestEvent struct exists, Server has requestEvents slice and eventsMu mutex</done>
</task>

<task type="auto">
  <name>Add recordRequest method and emit events from handleProxy</name>
  <files>internal/server/server.go</files>
  <action>
1. Create recordRequest method on Server:
   ```go
   func (s *Server) recordEvent(evt RequestEvent) {
       s.eventsMu.Lock()
       defer s.eventsMu.Unlock()
       s.requestEvents = append(s.requestEvents, evt)
       if len(s.requestEvents) > s.maxEvents {
           s.requestEvents = s.requestEvents[len(s.requestEvents)-s.maxEvents:]
       }
   }
   ```

2. Modify handleProxy to capture and emit events:
   - Create custom ResponseWriter wrapper to capture status code and bytes written
   - Record start time at beginning of handleProxy
   - After proxy.ServeHTTP, calculate latency and emit event:
     ```go
     start := time.Now()
     // ... setup proxy ...
     rw := &recordingResponseWriter{ResponseWriter: w}
     proxy.ServeHTTP(rw, r)
     latency := time.Since(start)
     
     s.recordEvent(RequestEvent{
         Time:     time.Now(),
         Method:   r.Method,
         Path:     r.URL.RequestURI(),
         Status:   rw.statusCode,
         Latency:  latency,
         Remote:   s.peerAddress(r),
         BytesIn:  int(r.ContentLength),
         BytesOut: rw.bytesWritten,
     })
     ```

3. Create recordingResponseWriter type:
   ```go
   type recordingResponseWriter struct {
       http.ResponseWriter
       statusCode   int
       bytesWritten int
   }
   
   func (rw *recordingResponseWriter) WriteHeader(code int) {
       rw.statusCode = code
       rw.ResponseWriter.WriteHeader(code)
   }
   
   func (rw *recordingResponseWriter) Write(b []byte) (int, error) {
       n, err := rw.ResponseWriter.Write(b)
       rw.bytesWritten += n
       return n, err
   }
   ```
  </action>
  <verify>grep -n "recordEvent\|recordingResponseWriter" internal/server/server.go | head -10</verify>
  <done>recordEvent method exists, handleProxy emits events with full request details</done>
</task>

<task type="auto">
  <name>Add /events HTTP endpoint to Handler</name>
  <files>internal/server/server.go</files>
  <action>
1. Create handleEvents method:
   ```go
   func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
       if r.Method != http.MethodGet {
           http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
           return
       }
       
       // Only allow access through tunnel (localhost)
       remoteAddr := s.peerAddress(r)
       isLocalhost := remoteAddr == "127.0.0.1" || remoteAddr == "::1" || remoteAddr == "localhost"
       if !isLocalhost {
           http.Error(w, "unauthorized", http.StatusForbidden)
           return
       }
       
       s.eventsMu.RLock()
       events := make([]RequestEvent, len(s.requestEvents))
       copy(events, s.requestEvents)
       s.eventsMu.RUnlock()
       
       w.Header().Set("Content-Type", "application/json")
       if err := json.NewEncoder(w).Encode(events); err != nil {
           s.log.Error().Err(err).Msg("failed to encode events")
       }
   }
   ```

2. Register the endpoint in Handler():
   ```go
   mux.HandleFunc("/events", s.handleEvents)
   ```

Place it alongside the other endpoints like /register, /healthz, etc.
  </action>
  <verify>grep -n "handleEvents\|/events" internal/server/server.go</verify>
  <done>/events endpoint registered and returns JSON array of request events</done>
</task>

</tasks>

<verification>
- [ ] RequestEvent struct matches tui.RequestLogMsg fields
- [ ] Server captures events after each proxied request
- [ ] Circular buffer limits to 100 events
- [ ] GET /events returns JSON array accessible through tunnel
- [ ] Only localhost can access /events endpoint
</verification>

<success_criteria>
Server successfully captures and exposes request events:
1. Each HTTP request through proxy generates a RequestEvent
2. Events include all required fields (timestamp, method, path, status, latency, remote IP, bytes)
3. Events endpoint returns last 100 events as JSON
4. Events are accessible via HTTP GET through the SSH tunnel
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-01-SUMMARY.md`
</output>
