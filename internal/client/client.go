package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rs/zerolog"
	"nhooyr.io/websocket"

	"github.com/gleicon/remo/internal/auth"
	"github.com/gleicon/remo/internal/identity"
	"github.com/gleicon/remo/internal/protocol"
	"github.com/gleicon/remo/internal/tui"
)

type Config struct {
	ServerURL    string
	Subdomain    string
	UpstreamURL  string
	Logger       zerolog.Logger
	DialTimeout  time.Duration
	Identity     *identity.Identity
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	EnableTUI    bool
}

type Client struct {
	cfg          Config
	log          zerolog.Logger
	upstream     *url.URL
	serverURL    *url.URL
	httpClient   *http.Client
	dialClient   *http.Client
	identity     *identity.Identity
	reconnectMin time.Duration
	reconnectMax time.Duration
	rand         *rand.Rand
	uiProgram    *tea.Program
	uiOnce       sync.Once
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
	serverURL, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("parse server url: %w", err)
	}
	upstreamURL, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream url: %w", err)
	}
	client := &Client{
		cfg:          cfg,
		log:          cfg.Logger,
		upstream:     upstreamURL,
		serverURL:    serverURL,
		httpClient:   &http.Client{Timeout: 20 * time.Second},
		dialClient:   &http.Client{Timeout: cfg.DialTimeout},
		identity:     cfg.Identity,
		reconnectMin: cfg.ReconnectMin,
		reconnectMax: cfg.ReconnectMax,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if cfg.EnableTUI {
		model := tui.NewModel(cfg.Subdomain)
		client.uiProgram = tea.NewProgram(model, tea.WithoutSignalHandler())
	}
	return client, nil
}

func (c *Client) Run(ctx context.Context) error {
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

func (c *Client) handleRequest(ctx context.Context, req *protocol.RequestPayload) (*protocol.ResponsePayload, error) {
	target, err := url.ParseRequestURI(req.Target)
	if err != nil {
		return nil, err
	}
	fullURL := *c.upstream
	fullURL.Path = target.Path
	fullURL.RawPath = target.RawPath
	fullURL.RawQuery = target.RawQuery
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL.String(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for key, values := range req.Headers {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, protocol.MaxBodyBytes))
	if err != nil {
		return nil, err
	}
	return &protocol.ResponsePayload{
		ID:      req.ID,
		Status:  resp.StatusCode,
		Headers: cloneHeader(resp.Header),
		Body:    body,
	}, nil
}

func cloneHeader(h http.Header) map[string][]string {
	result := make(map[string][]string, len(h))
	for k, values := range h {
		copyValues := make([]string, len(values))
		copy(copyValues, values)
		result[k] = copyValues
	}
	return result
}

func (c *Client) sendHello(ctx context.Context, conn *websocket.Conn) error {
	hello := &protocol.HelloPayload{
		Subdomain: c.cfg.Subdomain,
		PublicKey: base64.StdEncoding.EncodeToString(c.identity.Public),
		Timestamp: time.Now().Unix(),
	}
	message := auth.BuildHandshakeMessage(hello.Subdomain, hello.Timestamp)
	signature := ed25519.Sign(c.identity.Private, message)
	hello.Signature = base64.StdEncoding.EncodeToString(signature)
	if err := protocol.Write(ctx, conn, &protocol.Envelope{Type: protocol.TypeHello, Hello: hello}); err != nil {
		return err
	}
	ackCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ack, err := protocol.Read(ackCtx, conn)
	if err != nil {
		return err
	}
	if ack.Type != protocol.TypeReady {
		if ack.Error != "" {
			return fmt.Errorf("handshake rejected: %s", ack.Error)
		}
		return fmt.Errorf("unexpected handshake response: %s", ack.Type)
	}
	return nil
}

func (c *Client) runSession(ctx context.Context) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := c.dial(sessionCtx)
	if err != nil {
		return fmt.Errorf("dial tunnel: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "closing")
	if err := c.sendHello(sessionCtx, conn); err != nil {
		return err
	}
	c.log.Info().Str("subdomain", c.cfg.Subdomain).Msg("connected to server")
	c.sendUI(tui.StateMsg{Connected: true})
	go c.keepalive(sessionCtx, conn)
	for {
		env, err := protocol.Read(sessionCtx, conn)
		if err != nil {
			return err
		}
		if env.Type != protocol.TypeRequest || env.Request == nil {
			continue
		}
		start := time.Now()
		resp, err := c.handleRequest(sessionCtx, env.Request)
		latency := time.Since(start)
		var remote string
		if addrs, ok := env.Request.Headers["X-Forwarded-For"]; ok && len(addrs) > 0 {
			remote = addrs[len(addrs)-1]
		}
		if err != nil {
			c.log.Error().Err(err).Msg("handle request failed")
			_ = protocol.Write(sessionCtx, conn, &protocol.Envelope{
				Type: protocol.TypeResponse,
				Response: &protocol.ResponsePayload{
					ID:     env.Request.ID,
					Status: http.StatusBadGateway,
					Body:   []byte(err.Error()),
				},
			})
			c.sendUI(tui.RequestLogMsg{
				Time:     time.Now(),
				Method:   env.Request.Method,
				Path:     env.Request.Target,
				Status:   http.StatusBadGateway,
				Latency:  latency,
				Remote:   remote,
				BytesIn:  len(env.Request.Body),
				BytesOut: 0,
			})
			continue
		}
		if err := protocol.Write(sessionCtx, conn, &protocol.Envelope{Type: protocol.TypeResponse, Response: resp}); err != nil {
			return err
		}
		c.sendUI(tui.RequestLogMsg{
			Time:     time.Now(),
			Method:   env.Request.Method,
			Path:     env.Request.Target,
			Status:   resp.Status,
			Latency:  latency,
			Remote:   remote,
			BytesIn:  len(env.Request.Body),
			BytesOut: len(resp.Body),
		})
	}
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	tunnelURL := *c.serverURL
	switch tunnelURL.Scheme {
	case "http":
		tunnelURL.Scheme = "ws"
	case "https":
		tunnelURL.Scheme = "wss"
	}
	tunnelURL.Path = "/tunnel"
	q := tunnelURL.Query()
	q.Set("subdomain", c.cfg.Subdomain)
	tunnelURL.RawQuery = q.Encode()
	conn, _, err := websocket.Dial(ctx, tunnelURL.String(), &websocket.DialOptions{HTTPClient: c.dialClient})
	return conn, err
}

func (c *Client) keepalive(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := conn.Ping(pingCtx); err != nil {
				cancel()
				return
			}
			cancel()
		}
	}
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
	jit := time.Duration(c.rand.Int63n(int64(jitterRange)))
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
