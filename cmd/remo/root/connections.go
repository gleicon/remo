package root

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/state"
)

func newConnectionsCommand(r *rootCommand) *cobra.Command {
	return &cobra.Command{
		Use:   "connections",
		Short: "List all active connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnections(r)
		},
	}
}

func runConnections(r *rootCommand) error {
	s, err := state.New("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	conns := s.List()

	if len(conns) == 0 {
		fmt.Println("No active connections")
		return nil
	}

	// Print header
	fmt.Printf("%-15s %-10s %-12s %-10s\n", "Subdomain", "Status", "Uptime", "Port")
	fmt.Println(strings.Repeat("-", 50))

	// Print connections
	for _, c := range conns {
		statusIcon := "●"
		if c.Status != "ON" {
			statusIcon = "○"
		}
		fmt.Printf("%-15s %s %-9s %-12s %-10d\n",
			c.Subdomain,
			statusIcon,
			c.Status,
			formatUptime(c.Uptime),
			c.Port)
	}

	return nil
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
