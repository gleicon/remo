package root

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type statusOptions struct {
	server  string
	secret  string
	metrics bool
	timeout time.Duration
}

func newStatusCommand(r *rootCommand) *cobra.Command {
	opts := &statusOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Query remo server status or metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(r, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.server, "server", "http://127.0.0.1:18080", "base server url")
	cmd.Flags().StringVar(&opts.secret, "secret", "", "admin secret for authorization")
	cmd.Flags().BoolVar(&opts.metrics, "metrics", false, "fetch Prometheus metrics instead of JSON status")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 5*time.Second, "request timeout")
	return cmd
}

func runStatus(r *rootCommand, opts *statusOptions, output io.Writer) error {
	if opts.secret == "" {
		return errors.New("secret is required")
	}
	endpoint, err := buildStatusURL(opts.server, opts.metrics)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: opts.timeout}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+opts.secret)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if opts.metrics {
		_, err = io.Copy(output, resp.Body)
		return err
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	enc := json.NewEncoder(output)
	enc.SetIndent("", "  ")
	return enc.Encode(body)
}

func buildStatusURL(base string, metrics bool) (string, error) {
	if base == "" {
		return "", errors.New("server url is required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	path := "/status"
	if metrics {
		path = "/metrics"
	}
	if strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	u.Path = u.Path + path
	return u.String(), nil
}
