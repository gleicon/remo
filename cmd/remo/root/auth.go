package root

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/identity"
)

func newAuthCommand(r *rootCommand) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Identity utilities",
	}
	cmd.AddCommand(newAuthInitCommand(r))
	cmd.AddCommand(newAuthRotateCommand(r))
	return cmd
}

func newAuthInitCommand(r *rootCommand) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a local identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthInit(r, output)
		},
	}
	cmd.Flags().StringVar(&output, "out", identity.DefaultPath(), "identity output path")
	return cmd
}

func runAuthInit(r *rootCommand, path string) error {
	path = resolveIdentityPath(path)
	id, err := identity.Generate()
	if err != nil {
		return err
	}
	if err := id.Save(path); err != nil {
		return err
	}
	logger := r.Logger()
	logger.Info().Str("path", path).Msg("identity created")
	return nil
}

func newAuthRotateCommand(r *rootCommand) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate the local identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			path = resolveIdentityPath(path)
			backup := ""
			if _, err := os.Stat(path); err == nil {
				backup = fmt.Sprintf("%s.bak-%d", path, time.Now().Unix())
				if err := os.Rename(path, backup); err != nil {
					return fmt.Errorf("backup old identity: %w", err)
				}
			}
			id, err := identity.Generate()
			if err != nil {
				return err
			}
			if err := id.Save(path); err != nil {
				return err
			}
			logger := r.Logger()
			logger.Info().Str("path", path).Str("backup", backup).Str("public", base64.StdEncoding.EncodeToString(id.Public)).Msg("identity rotated")
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", identity.DefaultPath(), "identity file path")
	return cmd
}

func resolveIdentityPath(path string) string {
	if path == "" {
		path = identity.DefaultPath()
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".remo", "identity.json")
		}
	}
	return path
}
