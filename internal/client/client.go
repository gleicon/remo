package client

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"

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
	cfg          Config
	log          zerolog.Logger
	upstream     string
	reconnectMin time.Duration
	reconnectMax time.Duration
	uiProgram    *tea.Program
	uiOnce       sync.Once
	sshClient    *ssh.Client
	sshConn      <-chan ssh.NewChannel
	ctx          context.Context
	cancel       context.CancelFunc
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

func (c *Client) runSession(ctx context.Context) error {
	sshClient, err := c.dialSSH(ctx)
	if err != nil {
		return fmt.Errorf("dial ssh: %w", err)
	}
	c.sshClient = sshClient
	defer sshClient.Close()

	remotePort, err := c.setupReverseTunnel(ctx, sshClient)
	if err != nil {
		return fmt.Errorf("setup reverse tunnel: %w", err)
	}

	if err := c.register(ctx, sshClient, remotePort); err != nil {
		return fmt.Errorf("register with server: %w", err)
	}

	if c.cfg.URL != "" {
		c.log.Info().Str("url", c.cfg.URL).Str("subdomain", c.cfg.Subdomain).Int("port", remotePort).Msg("connected to server - tunnel ready")
	} else {
		c.log.Info().Str("subdomain", c.cfg.Subdomain).Int("port", remotePort).Msg("connected to server - tunnel ready")
	}
	c.sendUI(tui.StateMsg{Connected: true})

	<-ctx.Done()
	return ctx.Err()
}

func (c *Client) dialSSH(ctx context.Context) (*ssh.Client, error) {
	signer, err := ssh.NewSignerFromKey(c.cfg.Identity.Private)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            "remo",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.cfg.DialTimeout,
	}

	server := c.cfg.Server
	if c.cfg.ServerPort > 0 && c.cfg.ServerPort != 22 {
		server = fmt.Sprintf("%s:%d", server, c.cfg.ServerPort)
	} else if !strings.Contains(server, ":") {
		server = server + ":22"
	}

	c.log.Info().Str("server", server).Msg("dialing ssh")
	client, err := ssh.Dial("tcp", server, config)
	if err != nil {
		c.log.Error().Err(err).Str("server", server).Msg("ssh dial failed")
		return nil, fmt.Errorf("dial ssh: %w", err)
	}
	return client, nil
}

func (c *Client) setupReverseTunnel(ctx context.Context, client *ssh.Client) (int, error) {
	remotePort := c.cfg.RemotePort
	if remotePort <= 0 {
		remotePort = c.randomPort(8000, 9000)
	}

	localhost := "127.0.0.1"

	for i := 0; i < 10; i++ {
		listener, err := client.Listen("tcp", fmt.Sprintf("%s:%d", localhost, remotePort))
		if err == nil {
			c.log.Info().Int("port", remotePort).Msg("reverse tunnel listening")
			go c.handleTunnel(ctx, listener)
			return remotePort, nil
		}

		if strings.Contains(err.Error(), "port is already allocated") {
			remotePort = c.randomPort(8000, 9000)
			continue
		}
		return 0, fmt.Errorf("listen on remote: %w", err)
	}
	return 0, fmt.Errorf("could not find available port after 10 attempts")
}

func (c *Client) handleTunnel(ctx context.Context, listener net.Listener) {
	defer listener.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err.Error() == "use of closed network connection" {
				return
			}
			c.log.Error().Err(err).Msg("accept on tunnel failed")
			continue
		}
		go c.handleConnection(ctx, conn)
	}
}

func (c *Client) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	local, err := net.Dial("tcp", c.upstream)
	if err != nil {
		c.log.Error().Err(err).Msg("dial local upstream")
		return
	}
	defer local.Close()

	done := make(chan bool, 2)
	go func() {
		io.Copy(local, conn)
		done <- true
	}()
	go func() {
		io.Copy(conn, local)
		done <- true
	}()
	<-done
}

func (c *Client) register(ctx context.Context, sshClient *ssh.Client, remotePort int) error {
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

	httpClient := &http.Client{Timeout: 10 * time.Second}

	c.log.Info().Msg("registering through SSH tunnel")
	conn, err := sshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort))
	if err != nil {
		c.log.Warn().Err(err).Msg("failed to dial through tunnel, trying direct")
		resp, err := httpClient.Do(req)
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
	defer conn.Close()

	req.Write(conn)
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
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

func (c *Client) randomPort(min, max int) int {
	n, _ := big.NewInt(0), big.NewInt(0)
	n, _ = n.SetString(strconv.Itoa(max-min), 10)
	n, _ = n.SetString(strconv.Itoa(min), 10)
	return min + int(n.Int64())%((max-min)+1)
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
	jit := time.Duration(c.randomPort(0, int(jitterRange)))
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
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}
