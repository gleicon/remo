package root

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/client"
	"github.com/gleicon/remo/internal/identity"
)

type connectOptions struct {
	server     string
	subdomain  string
	upstream   string
	identity   string
	tui        bool
	remotePort int
	debug      bool
}

func newConnectCommand(r *rootCommand) *cobra.Command {
	opts := &connectOptions{}
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Expose a local service through remo",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnect(r, opts)
		},
	}
	cmd.Flags().StringVar(&opts.server, "server", "", "remo server (host or user@host:port)")
	cmd.Flags().StringVar(&opts.subdomain, "subdomain", "", "subdomain to claim")
	cmd.Flags().StringVar(&opts.upstream, "upstream", "http://127.0.0.1:3000", "local http service")
	cmd.Flags().StringVar(&opts.identity, "identity", identity.DefaultPath(), "path to identity file")
	cmd.Flags().BoolVar(&opts.tui, "tui", false, "enable terminal UI")
	cmd.Flags().IntVar(&opts.remotePort, "port", 0, "remote port to use (auto-assigned if not specified)")
	cmd.Flags().BoolVarP(&opts.debug, "debug", "v", false, "enable debug logging")
	return cmd
}

func runConnect(r *rootCommand, opts *connectOptions) error {
	if opts.server == "" {
		return fmt.Errorf("server is required (e.g., user@yourserver.com or yourserver.com)")
	}

	logger := r.Logger()
	if opts.debug {
		logger = logger.Level(zerolog.DebugLevel)
	}

	id, err := identity.Load(opts.identity)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	server, serverPort, err := parseServer(opts.server)
	if err != nil {
		return fmt.Errorf("parse server: %w", err)
	}

	clientCfg := client.Config{
		Server:      server,
		ServerPort:  serverPort,
		Subdomain:   opts.subdomain,
		UpstreamURL: opts.upstream,
		Logger:      logger,
		Identity:    id,
		EnableTUI:   opts.tui,
		RemotePort:  opts.remotePort,
	}
	cl, err := client.New(clientCfg)
	if err != nil {
		return err
	}
	return cl.Run(r.Context())
}

func parseServer(s string) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "ssh://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", 0, err
		}
		host = u.Hostname()
		portStr := u.Port()
		if portStr == "" {
			port = 22
		} else {
			port, err = strconv.Atoi(portStr)
			if err != nil {
				return "", 0, err
			}
		}
		return host, port, nil
	}

	parts := strings.Split(s, "@")
	if len(parts) == 2 {
		host = parts[1]
	} else {
		host = parts[0]
	}

	if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		end := strings.Index(host, "]")
		host = host[1:end]
	}

	if idx := strings.LastIndex(host, ":"); idx != -1 {
		portStr := host[idx+1:]
		host = host[:idx]
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port: %w", err)
		}
	} else {
		port = 22
	}

	if net.ParseIP(host) != nil || host == "localhost" {
		return host, port, nil
	}

	return host, port, nil
}
