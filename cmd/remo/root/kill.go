package root

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/state"
)

type killOptions struct {
	all bool
}

func newKillCommand(r *rootCommand) *cobra.Command {
	opts := &killOptions{}
	cmd := &cobra.Command{
		Use:   "kill [subdomain]",
		Short: "Kill a connection by subdomain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(r, opts, args)
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "kill all connections")
	return cmd
}

func runKill(r *rootCommand, opts *killOptions, args []string) error {
	s, err := state.New("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if opts.all {
		return killAll(s, r.Logger())
	}

	if len(args) == 0 {
		return fmt.Errorf("subdomain required (or use --all to kill all)")
	}

	subdomain := args[0]
	return killOne(s, subdomain)
}

func killOne(s *state.State, subdomain string) error {
	conn, exists := s.Get(subdomain)
	if !exists {
		return fmt.Errorf("connection '%s' not found", subdomain)
	}

	// Confirm with user
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Kill connection to '%s'? (y/n): ", subdomain)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Cancelled")
		return nil
	}

	// Kill the process
	if err := killProcess(conn.PID); err != nil {
		return fmt.Errorf("kill process: %w", err)
	}

	// Remove from state
	if err := s.Remove(subdomain); err != nil {
		return fmt.Errorf("remove from state: %w", err)
	}

	fmt.Printf("Connection to '%s' killed\n", subdomain)
	return nil
}

func killAll(s *state.State, log zerolog.Logger) error {
	conns := s.List()
	if len(conns) == 0 {
		fmt.Println("No connections to kill")
		return nil
	}

	// Confirm with user
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Kill all %d connections? (y/n): ", len(conns))
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Cancelled")
		return nil
	}

	// Kill each connection
	killed := 0
	failed := 0
	for _, conn := range conns {
		if err := killProcess(conn.PID); err != nil {
			log.Warn().Err(err).Str("subdomain", conn.Subdomain).Msg("failed to kill connection")
			failed++
		} else {
			killed++
		}
	}

	// Clear state
	if err := s.Clear(); err != nil {
		return fmt.Errorf("clear state: %w", err)
	}

	fmt.Printf("Killed %d connections (%d failed)\n", killed, failed)
	return nil
}

func killProcess(pid int) error {
	// Get process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	// Kill the process
	if err := process.Kill(); err != nil {
		return fmt.Errorf("kill process %d: %w", pid, err)
	}

	return nil
}
