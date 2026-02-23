package server

import (
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TunnelEntry represents an active tunnel with health tracking
type TunnelEntry struct {
	Subdomain string
	Port      int
	PubKey    string
	LastPing  time.Time
	CreatedAt time.Time
	SSHPID    int // SSH daemon process ID for this tunnel
}

// registry manages active tunnels with automatic cleanup
type registry struct {
	mu      sync.RWMutex
	active  map[string]*TunnelEntry
	timeout time.Duration
}

// newRegistry creates a new registry with the specified timeout for stale tunnels
func newRegistry(timeout time.Duration) *registry {
	if timeout <= 0 {
		timeout = 5 * time.Minute // Default 5 minute timeout
	}
	return &registry{
		active:  make(map[string]*TunnelEntry),
		timeout: timeout,
	}
}

// register adds a new tunnel to the registry
func (r *registry) register(subdomain string, port int, pubKey string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[subdomain]; exists {
		return false
	}

	// Try to find the SSH process for this tunnel
	sshpid := r.findSSHPID(port)

	r.active[subdomain] = &TunnelEntry{
		Subdomain: subdomain,
		Port:      port,
		PubKey:    pubKey,
		LastPing:  time.Now(),
		CreatedAt: time.Now(),
		SSHPID:    sshpid,
	}
	return true
}

// GetSSHPID returns the tracked SSH PID for a subdomain (for testing/debugging)
func (r *registry) GetSSHPID(subdomain string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.active[subdomain]; ok {
		return entry.SSHPID
	}
	return 0
}

// findSSHPID attempts to find the SSH daemon process ID handling a specific forwarded port
func (r *registry) findSSHPID(port int) int {
	// Use lsof to find the process listening on this specific port
	cmd := exec.Command("lsof", "-t", "-i", ":"+strconv.Itoa(port))
	output, err := cmd.Output()
	if err != nil {
		// Log error for debugging but don't fail
		return 0
	}
	if len(output) == 0 {
		return 0
	}

	pids := strings.Fields(string(output))
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		// Verify it's an sshd process
		cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
		psOutput, _ := cmd.Output()
		comm := strings.TrimSpace(string(psOutput))
		if strings.Contains(comm, "sshd") {
			return pid
		}
	}
	return 0
}

// unregister removes a tunnel from the registry
func (r *registry) unregister(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, subdomain)
}

// ping updates the last ping time for a tunnel, returns false if tunnel doesn't exist
func (r *registry) ping(subdomain string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.active[subdomain]
	if !ok {
		return false
	}
	entry.LastPing = time.Now()
	return true
}

// get retrieves tunnel information
func (r *registry) get(subdomain string) (port int, pubKey string, exists bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.active[subdomain]
	if !ok {
		return 0, "", false
	}
	return entry.Port, entry.PubKey, true
}

// has checks if a tunnel exists
func (r *registry) has(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.active[subdomain]
	return ok
}

// list returns all active subdomains
func (r *registry) list() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.active))
	for subdomain := range r.active {
		result = append(result, subdomain)
	}
	slices.Sort(result)
	return result
}

// listStale returns subdomains that haven't pinged within the timeout
func (r *registry) listStale() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	var stale []string
	for subdomain, entry := range r.active {
		if now.Sub(entry.LastPing) > r.timeout {
			stale = append(stale, subdomain)
		}
	}
	return stale
}

// cleanup removes stale tunnels and returns the count removed
func (r *registry) cleanup() int {
	stale := r.listStale()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, subdomain := range stale {
		delete(r.active, subdomain)
	}
	return len(stale)
}

// getStats returns statistics about active tunnels
func (r *registry) getStats() (total int, stale int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	total = len(r.active)
	for _, entry := range r.active {
		if now.Sub(entry.LastPing) > r.timeout {
			stale++
		}
	}
	return total, stale
}

// listByPubKey returns all tunnels belonging to a specific public key
func (r *registry) listByPubKey(pubKey string) []TunnelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []TunnelEntry
	for _, entry := range r.active {
		if entry.PubKey == pubKey {
			result = append(result, *entry)
		}
	}
	return result
}

// clearAll removes all tunnels from the registry and kills associated SSH processes
// Returns list of removed subdomains and executed kill commands
// If useSudo is true, kill commands will be prefixed with sudo
func (r *registry) clearAll(log func(string, ...interface{}), useSudo bool) ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := make([]string, 0, len(r.active))
	killCommands := make([]string, 0)

	for subdomain, entry := range r.active {
		removed = append(removed, subdomain)

		// Try to kill the SSH process
		if entry.SSHPID > 0 {
			killCmd := r.killSSHProcess(entry.SSHPID, subdomain, log, useSudo)
			if killCmd != "" {
				killCommands = append(killCommands, killCmd)
			}
		}

		// Also try to find and kill by port as fallback
		portKillCmd := r.killByPort(entry.Port, subdomain, log, useSudo)
		if portKillCmd != "" {
			killCommands = append(killCommands, portKillCmd)
		}

		delete(r.active, subdomain)
	}
	return removed, killCommands
}

// killSSHProcess kills a specific SSH process and returns the command used
func (r *registry) killSSHProcess(pid int, subdomain string, log func(string, ...interface{}), useSudo bool) string {
	if pid <= 0 {
		return ""
	}

	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.Command("sudo", "kill", "-9", strconv.Itoa(pid))
	} else {
		cmd = exec.Command("kill", "-9", strconv.Itoa(pid))
	}
	err := cmd.Run()

	cmdStr := "kill -9 " + strconv.Itoa(pid)
	if useSudo {
		cmdStr = "sudo " + cmdStr
	}

	if err != nil {
		if log != nil {
			log("Failed to kill SSH process for %s (PID %d): %v", subdomain, pid, err)
		}
		return cmdStr + " (FAILED: " + err.Error() + ")"
	}

	if log != nil {
		log("Killed SSH process for %s (PID %d)", subdomain, pid)
	}
	return cmdStr
}

// killByPort finds and kills SSH processes by the tunnel port
func (r *registry) killByPort(port int, subdomain string, log func(string, ...interface{}), useSudo bool) string {
	// Use lsof with sudo if needed to find processes listening on this port
	var lsofCmd *exec.Cmd
	if useSudo {
		lsofCmd = exec.Command("sudo", "lsof", "-t", "-i", ":"+strconv.Itoa(port))
	} else {
		lsofCmd = exec.Command("lsof", "-t", "-i", ":"+strconv.Itoa(port))
	}
	output, err := lsofCmd.Output()

	if err != nil || len(output) == 0 {
		return ""
	}

	pids := strings.Fields(string(output))
	killed := make([]string, 0)

	for _, pidStr := range pids {
		_, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Verify it's an sshd process (also with sudo if needed)
		var psCmd *exec.Cmd
		if useSudo {
			psCmd = exec.Command("sudo", "ps", "-p", pidStr, "-o", "comm=")
		} else {
			psCmd = exec.Command("ps", "-p", pidStr, "-o", "comm=")
		}
		psOutput, _ := psCmd.Output()
		if !strings.Contains(string(psOutput), "sshd") {
			continue
		}

		// Kill it
		var killCmd *exec.Cmd
		if useSudo {
			killCmd = exec.Command("sudo", "kill", "-9", pidStr)
		} else {
			killCmd = exec.Command("kill", "-9", pidStr)
		}
		killErr := killCmd.Run()

		if killErr != nil {
			if log != nil {
				log("Failed to kill SSH process on port %d for %s (PID %s): %v", port, subdomain, pidStr, killErr)
			}
			killed = append(killed, pidStr+"(FAILED)")
		} else {
			if log != nil {
				log("Killed SSH process on port %d for %s (PID %s)", port, subdomain, pidStr)
			}
			killed = append(killed, pidStr)
		}
	}

	cmdStr := "lsof -t -i :" + strconv.Itoa(port) + " | xargs kill -9"
	if useSudo {
		cmdStr = "sudo " + cmdStr
	}

	if len(killed) > 0 {
		return cmdStr + " (PIDs: " + strings.Join(killed, ", ") + ")"
	}
	return ""
}
