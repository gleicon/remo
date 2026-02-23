package root

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/auth"
	"github.com/gleicon/remo/internal/server"
	"github.com/gleicon/remo/internal/store"
	"github.com/rs/zerolog"
)

type serverOptions struct {
	listen          string
	domain          string
	subdomainPrefix string
	mode            string
	tlsCert         string
	tlsKey          string
	trusted         []string
	trustedHops     int
	authorized      string
	state           string
	autoReserve     bool
	allowRandom     bool
	adminSecret     string
	configPath      string
}

func newServerCommand(r *rootCommand) *cobra.Command {
	opts := &serverOptions{}
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run remo server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd, r, opts)
		},
	}
	cmd.Flags().StringVar(&opts.listen, "listen", ":8080", "listener address")
	cmd.Flags().StringVar(&opts.domain, "domain", "rempapps.site", "base domain")
	cmd.Flags().StringVar(&opts.subdomainPrefix, "subdomain-prefix", "", "subdomain prefix (e.g. apps for *.apps.domain)")
	cmd.Flags().StringVar(&opts.mode, "mode", string(server.ModeStandalone), "mode: standalone or behind-proxy")
	cmd.Flags().StringVar(&opts.tlsCert, "tls-cert", "", "path to TLS certificate")
	cmd.Flags().StringVar(&opts.tlsKey, "tls-key", "", "path to TLS private key")
	cmd.Flags().StringSliceVar(&opts.trusted, "trusted-proxy", nil, "trusted proxy CIDR (repeatable)")
	cmd.Flags().IntVar(&opts.trustedHops, "trusted-hops", 1, "max trusted proxy hops")
	cmd.Flags().StringVar(&opts.authorized, "authorized", "", "authorized keys file")
	cmd.Flags().StringVar(&opts.state, "state", defaultStatePath(), "path to SQLite state database")
	cmd.Flags().BoolVar(&opts.autoReserve, "reserve", false, "auto-reserve subdomain on connect")
	cmd.Flags().BoolVar(&opts.allowRandom, "allow-random", false, "allow clients to request random subdomains")
	cmd.Flags().StringVar(&opts.adminSecret, "admin-secret", "", "shared secret for admin endpoints")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "YAML config file")
	return cmd
}

func runServer(cmd *cobra.Command, r *rootCommand, opts *serverOptions) error {
	logger := r.Logger()
	ctx := r.Context()
	if opts.configPath != "" {
		cfg, err := loadServerConfig(opts.configPath)
		if err != nil {
			return err
		}
		applyServerConfig(cmd, opts, cfg)
	}
	serverMode := server.Mode(strings.ToLower(opts.mode))
	switch serverMode {
	case server.ModeStandalone, server.ModeProxy:
	default:
		return fmt.Errorf("invalid mode %s", opts.mode)
	}
	trustedCIDRs, err := parseProxyCIDRs(opts.trusted)
	if err != nil {
		return err
	}
	var st *store.Store
	if opts.state != "" {
		st, err = openState(opts.state)
		if err != nil {
			return err
		}
	}
	var authorizer *auth.AuthorizedKeys
	if st != nil {
		entries, err := st.AuthorizedEntries(ctx)
		if err != nil {
			st.Close()
			return err
		}
		if len(entries) > 0 {
			authorizer = auth.NewAuthorizedKeys(entries)
		}
	}
	if opts.authorized != "" {
		fileAuth, err := auth.LoadAuthorizedKeys(opts.authorized)
		if err != nil {
			if st != nil {
				st.Close()
			}
			return err
		}
		if st != nil {
			for _, entry := range fileAuth.Entries() {
				if err := st.UpsertAuthorizedKey(ctx, entry.Key, entry.Rule); err != nil {
					st.Close()
					return err
				}
			}
			entries, err := st.AuthorizedEntries(ctx)
			if err != nil {
				st.Close()
				return err
			}
			authorizer = auth.NewAuthorizedKeys(entries)
		} else {
			authorizer = fileAuth
		}
	}
	adminSecret, err := resolveAdminSecret(ctx, st, opts.adminSecret, logger)
	if err != nil {
		if st != nil {
			st.Close()
		}
		return err
	}
	srv := server.New(server.Config{
		Domain:          opts.domain,
		SubdomainPrefix: opts.subdomainPrefix,
		Logger:          logger,
		Authorizer:      authorizer,
		Mode:            serverMode,
		TLSCertFile:     opts.tlsCert,
		TLSKeyFile:      opts.tlsKey,
		TrustedProxies:  trustedCIDRs,
		TrustedHops:     opts.trustedHops,
		AdminSecret:     adminSecret,
		Store:           st,
		AutoReserve:     opts.autoReserve,
		AllowRandom:     opts.allowRandom,
	})
	err = srv.Run(ctx, opts.listen)
	if st != nil {
		st.Close()
	}
	// Don't return error for graceful shutdown
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func applyServerConfig(cmd *cobra.Command, opts *serverOptions, cfg *serverFileConfig) {
	if cfg == nil {
		return
	}
	flags := cmd.Flags()
	if cfg.Listen != "" && !flags.Changed("listen") {
		opts.listen = cfg.Listen
	}
	if cfg.Domain != "" && !flags.Changed("domain") {
		opts.domain = cfg.Domain
	}
	if cfg.SubdomainPrefix != "" && !flags.Changed("subdomain-prefix") {
		opts.subdomainPrefix = cfg.SubdomainPrefix
	}
	if cfg.Mode != "" && !flags.Changed("mode") {
		opts.mode = cfg.Mode
	}
	if cfg.TLSCert != "" && !flags.Changed("tls-cert") {
		opts.tlsCert = cfg.TLSCert
	}
	if cfg.TLSKey != "" && !flags.Changed("tls-key") {
		opts.tlsKey = cfg.TLSKey
	}
	if len(cfg.Trusted) > 0 && !flags.Changed("trusted-proxy") {
		opts.trusted = cfg.Trusted
	}
	if cfg.TrustedHops != 0 && !flags.Changed("trusted-hops") {
		opts.trustedHops = cfg.TrustedHops
	}
	if cfg.Authorized != "" && !flags.Changed("authorized") {
		opts.authorized = cfg.Authorized
	}
	if cfg.State != "" && !flags.Changed("state") {
		opts.state = cfg.State
	}
	if cfg.AutoReserve != nil && !flags.Changed("reserve") {
		opts.autoReserve = *cfg.AutoReserve
	}
	if cfg.AllowRandom != nil && !flags.Changed("allow-random") {
		opts.allowRandom = *cfg.AllowRandom
	}
	if cfg.AdminSecret != "" && !flags.Changed("admin-secret") {
		opts.adminSecret = cfg.AdminSecret
	}
}

func resolveAdminSecret(ctx context.Context, st *store.Store, provided string, logger zerolog.Logger) (string, error) {
	if provided != "" {
		return provided, nil
	}
	if st == nil {
		return "", errors.New("admin-secret is required (provide flag/config or enable --state for persistence)")
	}
	secret, err := st.GetSetting(ctx, "admin_secret")
	if err != nil {
		return "", err
	}
	if secret != "" {
		return secret, nil
	}
	secret, err = generateSecret()
	if err != nil {
		return "", err
	}
	if err := st.SetSetting(ctx, "admin_secret", secret); err != nil {
		return "", err
	}
	logger.Info().Msg("generated admin secret stored in state database")
	return secret, nil
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func parseProxyCIDRs(values []string) ([]*net.IPNet, error) {
	var result []*net.IPNet
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			_, cidr, err := net.ParseCIDR(value)
			if err != nil {
				return nil, err
			}
			result = append(result, cidr)
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid proxy address %s", value)
		}
		maskBits := 32
		if ip.To4() == nil {
			maskBits = 128
		}
		cidr := &net.IPNet{IP: ip, Mask: net.CIDRMask(maskBits, maskBits)}
		result = append(result, cidr)
	}
	return result, nil
}
