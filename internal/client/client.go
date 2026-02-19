// Package client provides the SSH tunnel client functionality.
// Uses system ssh command with -R for reverse tunneling instead of golang.org/x/crypto/ssh
// to avoid GatewayPorts requirement issues.
package client

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rs/zerolog"

	"github.com/gleicon/remo/internal/identity"
	"github.com/gleicon/remo/internal/tui"
)

type Config struct {
	Server       string
	ServerPort   int
	Subdomain    string
	URL          string
	UpstreamURL  string
	Logger       zerolog.Logger
	DialTimeout  time.Duration
	Identity     *identity.Identity
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	EnableTUI    bool
	RemotePort   int
}

type Client struct {
	cfg            Config
	log            zerolog.Logger
	upstream       string
	reconnectMin   time.Duration
	reconnectMax   time.Duration
	uiProgram      *tea.Program
	uiOnce         sync.Once
	ctx            context.Context
	cancel         context.CancelFunc
	sshCmd         *exec.Cmd
	eventsClient   *http.Client
	pollInterval   time.Duration
	lastEventIndex int // Track which events we've already sent
}

func New(cfg Config) (*Client, error) {
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 15 * time.Second
	}
	if cfg.Identity == nil {
		return nil, fmt.Errorf("identity is required")
	}
	if cfg.ReconnectMin <= 0 {
		cfg.ReconnectMin = time.Second
	}
	if cfg.ReconnectMax <= 0 {
		cfg.ReconnectMax = 30 * time.Second
	}
	if cfg.ReconnectMax < cfg.ReconnectMin {
		cfg.ReconnectMax = cfg.ReconnectMin
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		cfg:          cfg,
		log:          cfg.Logger,
		upstream:     cfg.UpstreamURL,
		reconnectMin: cfg.ReconnectMin,
		reconnectMax: cfg.ReconnectMax,
		ctx:          ctx,
		cancel:       cancel,
		eventsClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		pollInterval: time.Second, // Poll every second per locked decision
	}
	if cfg.EnableTUI {
		model := tui.NewModel(cfg.Subdomain)
		client.uiProgram = tea.NewProgram(model, tea.WithoutSignalHandler())
	}
	return client, nil
}

func (c *Client) Run(ctx context.Context) error {
	c.ctx = ctx
	c.startUI()

	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.runSession(ctx)
		if err == nil {
			attempt = 0
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		attempt++
		delay := c.backoffDuration(attempt)
		c.log.Warn().Err(err).Int("attempt", attempt).Dur("backoff", delay).Msg("session ended; reconnecting")
		c.sendUI(tui.StateMsg{Connected: false, Attempt: attempt, Backoff: delay, Err: err.Error()})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// parsePortFromOutput extracts the allocated remote port from SSH verbose output.
// Looks for patterns like "Allocated port 12345 for remote forward to 127.0.0.1:18080"
var portRegex = regexp.MustCompile(`Allocated port (\d+) for remote forward`)

func parsePortFromOutput(line string) (int, error) {
	matches := portRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, fmt.Errorf("no port found in output")
	}
	port, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %w", err)
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}
	return port, nil
}

// monitorSSH reads SSH verbose output to find the allocated port and waits for process exit.
func (c *Client) monitorSSH(stdout, stderr io.Reader, portChan chan<- int, done chan<- error) {
	var portFound bool
	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))

	for scanner.Scan() {
		line := scanner.Text()
		c.log.Debug().Str("ssh_output", line).Msg("ssh")

		if !portFound {
			if port, err := parsePortFromOutput(line); err == nil {
				portFound = true
				select {
				case portChan <- port:
				default:
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		done <- fmt.Errorf("ssh output scanner error: %w", err)
		return
	}

	// Process exited
	done <- fmt.Errorf("ssh process exited")
}

func (c *Client) runSession(ctx context.Context) error {
	// Get identity key path - save to temp file for ssh -i
	keyPath, err := c.saveIdentityKey()
	if err != nil {
		return fmt.Errorf("save identity key: %w", err)
	}
	defer os.Remove(keyPath)

	// Build SSH command
	server := c.cfg.Server
	if c.cfg.ServerPort > 0 && c.cfg.ServerPort != 22 {
		server = fmt.Sprintf("%s:%d", server, c.cfg.ServerPort)
	}

	// Build ssh arguments
	// -v: verbose output for port parsing
	// -N: don't execute remote command
	// -R 0:localhost:18080: reverse tunnel with auto port allocation
	// -o StrictHostKeyChecking=no: accept any host key
	// -i: identity file path
	args := []string{
		"-v",
		"-N",
		"-R", fmt.Sprintf("0:localhost:%s", c.upstreamPort()),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-i", keyPath,
		fmt.Sprintf("remo@%s", server),
	}

	c.log.Info().Str("server", server).Str("upstream", c.upstream).Msg("starting ssh tunnel")

	cmd := exec.CommandContext(ctx, "ssh", args...)
	c.sshCmd = cmd

	// Get stdout and stderr pipes for port parsing
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ssh stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ssh stderr pipe: %w", err)
	}

	// Start the SSH process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ssh: %w", err)
	}

	// Channel to receive allocated port
	portChan := make(chan int, 1)
	doneChan := make(chan error, 1)

	// Start goroutine to monitor SSH output
	go c.monitorSSH(stdout, stderr, portChan, doneChan)

	// Wait for port allocation with timeout
	var remotePort int
	select {
	case remotePort = <-portChan:
		c.log.Info().Int("port", remotePort).Msg("ssh allocated remote port")
	case err := <-doneChan:
		cmd.Process.Kill()
		return fmt.Errorf("ssh exited before port allocation: %w", err)
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return fmt.Errorf("timeout waiting for port allocation")
	case <-ctx.Done():
		cmd.Process.Kill()
		return ctx.Err()
	}

	// Register with the server through the tunnel
	if err := c.register(ctx, remotePort); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("register with server: %w", err)
	}

	// Start polling for request events
	c.startEventPolling(ctx)

	if c.cfg.URL != "" {
		c.log.Info().Str("url", c.cfg.URL).Str("subdomain", c.cfg.Subdomain).Int("port", remotePort).Msg("connected to server - tunnel ready")
	} else {
		c.log.Info().Str("subdomain", c.cfg.Subdomain).Int("port", remotePort).Msg("connected to server - tunnel ready")
	}
	c.sendUI(tui.StateMsg{Connected: true})

	// Wait for SSH process to exit (this blocks until reconnection needed)
	select {
	case err := <-doneChan:
		return err
	case <-ctx.Done():
		cmd.Process.Kill()
		cmd.Wait()
		return ctx.Err()
	}
}

// upstreamPort extracts the port from the upstream URL
func (c *Client) upstreamPort() string {
	// Parse "http://localhost:18080" -> "18080"
	parts := strings.Split(c.upstream, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	// Default port
	if strings.HasPrefix(c.upstream, "https") {
		return "443"
	}
	return "80"
}

// saveIdentityKey saves the Ed25519 private key to a temp file for ssh -i
func (c *Client) saveIdentityKey() (string, error) {
	// Create temp file
	f, err := os.CreateTemp("", "remo-identity-*.key")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	// Write OpenSSH format private key
	keyData, err := c.cfg.Identity.MarshalPrivateKey()
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}

	if _, err := f.Write(keyData); err != nil {
		return "", fmt.Errorf("write key: %w", err)
	}

	// Set strict permissions (required by SSH)
	if err := f.Chmod(0600); err != nil {
		return "", fmt.Errorf("chmod key file: %w", err)
	}

	return f.Name(), nil
}

func (c *Client) register(ctx context.Context, remotePort int) error {
	publicKeyBase64 := base64.StdEncoding.EncodeToString(c.cfg.Identity.Public)

	reg := struct {
		Subdomain  string `json:"subdomain"`
		RemotePort int    `json:"remote_port"`
	}{
		Subdomain:  c.cfg.Subdomain,
		RemotePort: remotePort,
	}

	body, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}

	req, err := http.NewRequest("POST", "http://127.0.0.1:18080/register", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remo-Publickey", publicKeyBase64)

	c.log.Info().Msg("registering through SSH tunnel")

	// Use a client with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed: %s", string(body))
	}

	return c.handleRegisterResponse(resp)
}

func (c *Client) handleRegisterResponse(resp *http.Response) error {
	var result struct {
		Subdomain string `json:"subdomain"`
		URL       string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.log.Warn().Err(err).Msg("failed to decode registration response")
		return nil
	}
	if result.Subdomain != "" {
		c.cfg.Subdomain = result.Subdomain
	}
	if result.URL != "" {
		c.cfg.URL = result.URL
		c.log.Info().Str("url", result.URL).Msg("tunnel ready - use this URL to access your service")
		c.sendUI(tui.URLMsg{URL: result.URL})
	}
	return nil
}

func (c *Client) backoffDuration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := c.reconnectMin
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= c.reconnectMax {
			delay = c.reconnectMax
			break
		}
	}
	jitterRange := delay / 4
	if jitterRange <= 0 {
		return delay
	}
	// Use simple random jitter instead of crypto/rand
	jit := time.Duration(int(jitterRange) / 2) // Simple jitter
	return delay + jit
}

func (c *Client) startUI() {
	if c.uiProgram == nil {
		return
	}
	c.uiOnce.Do(func() {
		go func() {
			if err := c.uiProgram.Start(); err != nil {
				c.log.Error().Err(err).Msg("tui exited")
			}
		}()
	})
}

func (c *Client) sendUI(msg tea.Msg) {
	if c.uiProgram == nil {
		return
	}
	c.uiProgram.Send(msg)
}

func (c *Client) Close() error {
	c.cancel()
	if c.sshCmd != nil && c.sshCmd.Process != nil {
		c.sshCmd.Process.Kill()
		c.sshCmd.Wait()
	}
	return nil
}

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
