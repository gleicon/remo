---
phase: 01-ssh-client-rewrite
plan: 01
subsystem: client
tags: [ssh, exec, subprocess, port-parsing, reconnection]

# Dependency graph
requires: []
provides:
  - SSH client using exec.Command instead of golang.org/x/crypto/ssh
  - Port parsing from SSH verbose output
  - Automatic reconnection on SSH process failure
  - OpenSSH format private key export
affects:
  - cmd/remo/root/connect.go
  - All client connection flows

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Subprocess-based SSH with port parsing
    - Regex-based log parsing for port extraction
    - OpenSSH private key format encoding

key-files:
  created: []
  modified:
    - internal/client/client.go - Rewritten to use exec.Command
    - internal/identity/identity.go - Added MarshalPrivateKey method
    - internal/client/client_test.go - Added port parsing tests
    - internal/identity/identity_test.go - Added key export tests
    - go.mod - Cleaned up unused dependencies

key-decisions:
  - "Used exec.Command instead of ssh.Dial to avoid GatewayPorts requirement"
  - "Implemented OpenSSH private key format export for ssh -i compatibility"
  - "Removed handleTunnel/handleConnection - system SSH now handles tunneling"

patterns-established:
  - "Port extraction: Use regex to parse 'Allocated port N' from SSH verbose output"
  - "Process monitoring: Goroutine reads stdout/stderr and signals on process exit"
  - "Key export: Ed25519 keys encoded in OpenSSH format for system SSH compatibility"

requirements-completed:
  - CLI-01
  - CLI-02
  - CLI-03
  - CLI-04
  - CLI-05
  - CLI-06

# Metrics
duration: 4min
completed: 2026-02-19T00:27:03Z
---

# Phase 01 Plan 01: SSH Client Rewrite Summary

**SSH client rewritten to use system ssh command with -R for reverse tunneling, eliminating GatewayPorts requirement. Port auto-detected from verbose output with regex parsing.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-02-19T00:22:56Z
- **Completed:** 2026-02-19T00:27:03Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- Replaced ssh.Dial-based implementation with exec.Command("ssh", ...) subprocess approach
- Implemented parsePortFromOutput() with regex to extract allocated port from SSH verbose output
- Added monitorSSH() goroutine to watch process and trigger reconnections on exit
- Added MarshalPrivateKey() to export Ed25519 keys in OpenSSH format for ssh -i flag
- Removed 127 lines of SSH dialer/tunnel handling code (handleTunnel, handleConnection, dialSSH, setupReverseTunnel)
- Added 12 test cases for port parsing covering various SSH output formats and edge cases

## Task Commits

Each task was committed atomically:

1. **Task 1: Refactor client to use external SSH command** - `cfc382a` (feat)
2. **Task 2: Add port parsing tests and validate reconnection** - `0d210a5` (test)
3. **Task 3: Verify go.mod cleanup and final validation** - `17174be` (chore)

**Plan metadata:** [pending]

## Files Created/Modified

- `internal/client/client.go` - Complete rewrite using exec.Command, removed ssh.Dial dependency
- `internal/identity/identity.go` - Added MarshalPrivateKey() for OpenSSH format export
- `internal/client/client_test.go` - Added TestParsePortFromOutput and TestUpstreamPort
- `internal/identity/identity_test.go` - Added TestMarshalPrivateKey and TestMarshalPrivateKeyNil
- `go.mod` - golang.org/x/crypto/ssh dependency removed

## Decisions Made

1. **Subprocess approach over library** - System SSH handles GatewayPorts internally, avoiding the hang issue
2. **OpenSSH key format** - Required for ssh -i flag compatibility; implemented full wire format encoding
3. **Removed tunnel handlers** - System SSH now handles connection forwarding, simplifying client code
4. **Port validation** - parsePortFromOutput validates port is in valid range (1-65535)

## Deviations from Plan

**[Rule 2 - Missing Critical] Added OpenSSH private key format export**
- **Found during:** Task 1 implementation
- **Issue:** ssh -i flag requires OpenSSH format, but identity package only stored raw Ed25519 bytes
- **Fix:** Implemented MarshalPrivateKey() with full OpenSSH wire format encoding including magic header, cipher/kdf info, and PEM structure
- **Files modified:** internal/identity/identity.go (+100 lines)
- **Verification:** TestMarshalPrivateKey verifies PEM format and base64 decoding
- **Committed in:** cfc382a (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Essential for SSH authentication to work with subprocess approach. No scope creep.

## Issues Encountered

None - implementation proceeded smoothly.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Client foundation complete, ready for TUI improvements (Phase 2)
- Subprocess-based SSH approach validated
- Port parsing and reconnection logic tested
- Identity export format established

---
*Phase: 01-ssh-client-rewrite*
*Completed: 2026-02-19*
