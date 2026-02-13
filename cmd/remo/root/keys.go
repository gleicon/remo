package root

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newKeysCommand(r *rootCommand) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage authorized keys",
	}
	cmd.AddCommand(newKeysListCommand(r))
	cmd.AddCommand(newKeysAddCommand(r))
	cmd.AddCommand(newKeysRemoveCommand(r))
	return cmd
}

func newKeysListCommand(r *rootCommand) *cobra.Command {
	var statePath string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List authorized keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openState(statePath)
			if err != nil {
				return err
			}
			defer st.Close()
			entries, err := st.AuthorizedEntries(cmd.Context())
			if err != nil {
				return err
			}
			for _, entry := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", base64.StdEncoding.EncodeToString(entry.Key), entry.Rule)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", defaultStatePath(), "state db path")
	return cmd
}

func newKeysAddCommand(r *rootCommand) *cobra.Command {
	var statePath, keyValue, rule, prefix string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update an authorized key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyValue == "" {
				return fmt.Errorf("pubkey is required")
			}
			decoded, err := base64.StdEncoding.DecodeString(keyValue)
			if err != nil {
				return fmt.Errorf("invalid pubkey: %w", err)
			}
			if len(decoded) != ed25519.PublicKeySize {
				return fmt.Errorf("pubkey must be base64 ed25519 public key")
			}
			st, err := openState(statePath)
			if err != nil {
				return err
			}
			defer st.Close()
			if prefix != "" && rule != "*" {
				return fmt.Errorf("use either --rule or --prefix, not both")
			}
			if prefix != "" {
				rule = fmt.Sprintf("%s*", strings.TrimSuffix(prefix, "*"))
			}
			if rule == "" {
				rule = "*"
			}
			if err := st.UpsertAuthorizedKey(cmd.Context(), ed25519.PublicKey(decoded), rule); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "key stored (%s)\n", rule)
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", defaultStatePath(), "state db path")
	cmd.Flags().StringVar(&keyValue, "pubkey", "", "base64 ed25519 public key")
	cmd.Flags().StringVar(&rule, "rule", "*", "allowed subdomain rule (foo-* or *)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "shortcut for setting rule prefix-*")
	return cmd
}

func newKeysRemoveCommand(r *rootCommand) *cobra.Command {
	var statePath, keyValue string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an authorized key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyValue == "" {
				return fmt.Errorf("pubkey is required")
			}
			st, err := openState(statePath)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.DeleteAuthorizedKey(cmd.Context(), keyValue); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "key removed")
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", defaultStatePath(), "state db path")
	cmd.Flags().StringVar(&keyValue, "pubkey", "", "base64 ed25519 public key")
	return cmd
}
