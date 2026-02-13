package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/gleicon/remo/internal/auth"
	"github.com/gleicon/remo/internal/client"
	"github.com/gleicon/remo/internal/identity"
	"github.com/gleicon/remo/internal/server"
)

func TestHTTPTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := zerolog.New(io.Discard)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusCreated)
		w.Write(append([]byte("echo:"), body...))
	}))
	defer backend.Close()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	authFile := filepath.Join(t.TempDir(), "authorized")
	encoded := base64.StdEncoding.EncodeToString(id.Public)
	if err := os.WriteFile(authFile, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatalf("write authorized file: %v", err)
	}
	authorizer, err := auth.LoadAuthorizedKeys(authFile)
	if err != nil {
		t.Fatalf("load authorized keys: %v", err)
	}
	srv := server.New(server.Config{Domain: "rempapps.site", Logger: logger, Authorizer: authorizer, Mode: server.ModeProxy, AdminSecret: "secret"})
	public := httptest.NewServer(srv.Handler())
	defer public.Close()
	clientCfg := client.Config{
		ServerURL:   public.URL,
		Subdomain:   "demo",
		UpstreamURL: backend.URL,
		Logger:      logger,
		Identity:    id,
	}
	cli, err := client.New(clientCfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	clientCtx, clientCancel := context.WithCancel(ctx)
	defer clientCancel()
	go func() {
		if err := cli.Run(clientCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("client run error: %v", err)
		}
	}()
	waitForTunnel(t, srv, "demo")
	req, err := http.NewRequest(http.MethodPost, public.URL+"/hook", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Host = "demo.rempapps.site"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Upstream") != "ok" {
		t.Fatalf("missing upstream header")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(body) != "echo:hello" {
		t.Fatalf("unexpected body: %s", string(body))
	}
	statusReq, err := http.NewRequest(http.MethodGet, public.URL+"/status", nil)
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	statusReq.Header.Set("Authorization", "Bearer secret")
	statusResp, err := http.DefaultClient.Do(statusReq)
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status code: %d", statusResp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if tunnels, ok := payload["active_tunnels"].(float64); !ok || tunnels < 1 {
		t.Fatalf("expected active tunnels")
	}
}

func waitForTunnel(t *testing.T, srv *server.Server, subdomain string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.HasTunnel(subdomain) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("tunnel %s not ready", subdomain)
}
