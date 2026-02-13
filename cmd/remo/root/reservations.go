package root

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newReservationsCommand(r *rootCommand) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reservations",
		Short: "Manage subdomain reservations",
	}
	cmd.AddCommand(newReservationsListCommand(r))
	cmd.AddCommand(newReservationsSetCommand(r))
	return cmd
}

func newReservationsListCommand(r *rootCommand) *cobra.Command {
	var statePath string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reservations",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openState(statePath)
			if err != nil {
				return err
			}
			defer st.Close()
			reservations, err := st.Reservations(cmd.Context())
			if err != nil {
				return err
			}
			for _, res := range reservations {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", res.Subdomain, res.Pubkey, res.CreatedAt.Format(time.RFC3339))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", defaultStatePath(), "state db path")
	return cmd
}

func newReservationsSetCommand(r *rootCommand) *cobra.Command {
	var statePath, subdomain, pubkey string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set reservation owner",
		RunE: func(cmd *cobra.Command, args []string) error {
			if subdomain == "" {
				return fmt.Errorf("subdomain is required")
			}
			if pubkey == "" {
				return fmt.Errorf("pubkey is required")
			}
			if _, err := base64.StdEncoding.DecodeString(pubkey); err != nil {
				return fmt.Errorf("invalid pubkey: %w", err)
			}
			st, err := openState(statePath)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.ReserveSubdomain(cmd.Context(), subdomain, pubkey); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reserved %s\n", subdomain)
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", defaultStatePath(), "state db path")
	cmd.Flags().StringVar(&subdomain, "subdomain", "", "subdomain to reserve")
	cmd.Flags().StringVar(&pubkey, "pubkey", "", "base64 ed25519 public key")
	return cmd
}
