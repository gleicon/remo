package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/client"
	"github.com/gleicon/remo/internal/identity"
)

type connectOptions struct {
	serverURL string
	subdomain string
	upstream  string
	identity  string
	tui       bool
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
	cmd.Flags().StringVar(&opts.serverURL, "server", "http://127.0.0.1:8080", "remo server url")
	cmd.Flags().StringVar(&opts.subdomain, "subdomain", "", "subdomain to claim")
	cmd.Flags().StringVar(&opts.upstream, "upstream", "http://127.0.0.1:3000", "local http service")
	cmd.Flags().StringVar(&opts.identity, "identity", identity.DefaultPath(), "path to identity file")
	cmd.Flags().BoolVar(&opts.tui, "tui", false, "enable terminal UI")
	return cmd
}

func runConnect(r *rootCommand, opts *connectOptions) error {
	if opts.subdomain == "" {
		return fmt.Errorf("subdomain is required")
	}
	id, err := identity.Load(opts.identity)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}
	clientCfg := client.Config{
		ServerURL:   opts.serverURL,
		Subdomain:   opts.subdomain,
		UpstreamURL: opts.upstream,
		Logger:      r.Logger(),
		Identity:    id,
		EnableTUI:   opts.tui,
	}
	cl, err := client.New(clientCfg)
	if err != nil {
		return err
	}
	return cl.Run(r.Context())
}
