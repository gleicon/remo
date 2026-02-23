package server

import (
	"os"
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

// clearAll removes all tunnels from the registry and kills ALL remo-owned SSH processes
// Returns list of removed subdomains and executed kill commands
// Uses pgrep to find all remo-owned sshd processes and kills them all at once
func (r *registry) clearAll(log func(string, ...interface{}), useSudo bool) ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := make([]string, 0, len(r.active))
	for subdomain := range r.active {
		removed = append(removed, subdomain)
		delete(r.active, subdomain)
	}

	// Kill ALL remo-owned sshd processes at once - simple and effective
	killCmd := r.killAllRemoSSH(log, useSudo)
	killCommands := []string{}
	if killCmd != "" {
		killCommands = append(killCommands, killCmd)
	}

	return removed, killCommands
}

// killSSHProcess kills a specific SSH process owned by the same user
// Returns the command used. Only uses sudo for discovery, not for kill.
func (r *registry) killSSHProcess(pid int, subdomain string, log func(string, ...interface{}), useSudo bool) string {
	if pid <= 0 {
		return ""
	}

	// Verify process is owned by current user before killing
	owner, err := r.getProcessOwner(pid)
	if err != nil {
		if log != nil {
			log("Cannot determine owner of PID %d for %s: %v", pid, subdomain, err)
		}
		return ""
	}

	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = "remo" // fallback
	}

	if owner != currentUser {
		if log != nil {
			log("PID %d for %s is owned by %s, not %s - skipping kill", pid, subdomain, owner, currentUser)
		}
		return ""
	}

	// Kill without sudo since we own the process
	cmd := exec.Command("kill", "-9", strconv.Itoa(pid))
	err = cmd.Run()

	cmdStr := "kill -9 " + strconv.Itoa(pid)
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

// getProcessOwner returns the username that owns a process
func (r *registry) getProcessOwner(pid int) (string, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "user=")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// killAllRemoSSH kills ALL sshd processes owned by the remo user
// NO SUDO NEEDED - remo user kills its own processes
// This is the simplest and most effective approach - when remo stops, all remo SSH tunnels die
func (r *registry) killAllRemoSSH(log func(string, ...interface{}), useSudo bool) string {
	// Use pgrep to find all sshd processes owned by remo (no sudo needed to discover own processes)
	cmd := exec.Command("pgrep", "-u", "remo", "-x", "sshd")

	output, err := cmd.Output()
	if err != nil {
		// No processes found or error
		return ""
	}

	pids := strings.Fields(string(output))
	if len(pids) == 0 {
		return ""
	}

	// Kill all PIDs at once (no sudo - remo kills its own processes)
	args := []string{"-9"}
	args = append(args, pids...)
	killCmd := exec.Command("kill", args...)

	killErr := killCmd.Run()

	cmdStr := "pgrep -u remo -x sshd | xargs kill -9"

	if killErr != nil {
		if log != nil {
			log("Killed %d remo-owned sshd processes (PIDs: %s) - some may have failed: %v",
				len(pids), strings.Join(pids, ", "), killErr)
		}
		return cmdStr + " (PIDs: " + strings.Join(pids, ", ") + ", some FAILED: " + killErr.Error() + ")"
	}

	if log != nil {
		log("Killed all %d remo-owned sshd processes (PIDs: %s)", len(pids), strings.Join(pids, ", "))
	}
	return cmdStr + " (PIDs: " + strings.Join(pids, ", ") + ")"
}

// killByPort finds and kills SSH processes by the tunnel port
// SAFETY: Only uses sudo for discovery (lsof), NOT for kill. Verifies process ownership.
func (r *registry) killByPort(port int, subdomain string, log func(string, ...interface{}), useSudo bool) string {
	// Use lsof with sudo for discovery only (to find all processes on the port)
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
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = "remo"
	}

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Verify it's an sshd process
		psCmd := exec.Command("ps", "-p", pidStr, "-o", "comm=")
		psOutput, _ := psCmd.Output()
		if !strings.Contains(string(psOutput), "sshd") {
			continue
		}

		// CRITICAL: Verify we own this process before killing (security!)
		owner, err := r.getProcessOwner(pid)
		if err != nil {
			if log != nil {
				log("Cannot verify owner of PID %s on port %d for %s - skipping", pidStr, port, subdomain)
			}
			continue
		}

		if owner != currentUser {
			if log != nil {
				log("PID %s on port %d is owned by %s, not %s - skipping kill", pidStr, port, owner, currentUser)
			}
			continue
		}

		// Kill WITHOUT sudo since we verified ownership
		killCmd := exec.Command("kill", "-9", pidStr)
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
		cmdStr = "sudo " + cmdStr + " (discovery only, kill without sudo)"
	}

	if len(killed) > 0 {
		return cmdStr + " (PIDs: " + strings.Join(killed, ", ") + ")"
	}
	return ""
}
