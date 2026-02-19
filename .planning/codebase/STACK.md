# Technology Stack

**Analysis Date:** 2026-02-18

## Languages

**Primary:**
- Go 1.25.3 - All application code (CLI, server, client)

## Runtime

**Environment:**
- Go runtime (compiled binary)

**Package Manager:**
- Go modules (go.mod/go.sum)
- Lockfile: present (go.sum)

## Frameworks

**CLI:**
- spf13/cobra v1.9.1 - CLI framework for command structure

**TUI (Terminal UI):**
- charmbracelet/bubbletea v0.24.2 - Interactive terminal UI
- charmbracelet/bubbles v0.16.1 - TUI components
- charmbracelet/lipgloss v0.10.0 - Terminal styling

**Networking:**
- golang.org/x/crypto v0.48.0 - SSH protocol support

**Data:**
- modernc.org/sqlite v1.29.6 - SQLite database driver (pure Go)

**Logging:**
- rs/zerolog v1.33.0 - Structured logging

**Configuration:**
- gopkg.in/yaml.v3 v3.0.1 - YAML parsing

## Key Dependencies

**Critical:**
- spf13/cobra v1.9.1 - CLI command framework
- charmbracelet/bubbletea v0.24.2 - TUI rendering
- golang.org/x/crypto v0.48.0 - SSH client/server implementation
- modernc.org/sqlite v1.29.6 - Persistent storage for authorized keys and reservations

**Infrastructure:**
- rs/zerolog v1.33.0 - Logging with structured output
- gopkg.in/yaml.v3 v3.0.1 - Configuration file parsing

## Configuration

**Environment:**
- Configuration via CLI flags and YAML files
- Environment variable: `REMO_LOG` for log level
- No .env file support (stateless design)

**Build:**
- GoReleaser configuration: `.goreleaser.yaml`
- CGO disabled for cross-platform builds
- Targets: Linux, Windows, Darwin (macOS)
- Architectures: amd64, arm64

**Build Configuration:**
```yaml
# .goreleaser.yaml
goos: [linux, windows, darwin]
goarch: [amd64, arm64]
CGO_ENABLED=0
```

## Platform Requirements

**Development:**
- Go 1.25.3+
- Make (for build automation)

**Production:**
- Binary deployment (no runtime dependencies)
- SQLite for persistence (optional)
- SSH server access (for tunnel client)

---

*Stack analysis: 2026-02-18*
