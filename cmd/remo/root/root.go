package root

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/logging"
)

var Version = "dev"

type rootCommand struct {
	ctx      context.Context
	logLevel string
	logger   zerolog.Logger
}

func NewRootCommand(ctx context.Context) *cobra.Command {
	r := &rootCommand{ctx: ctx}
	cmd := &cobra.Command{
		Use:     "remo",
		Short:   "Self-hosted reverse tunnel",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Always initialize logger to ensure it's properly set up
			level := r.logLevel
			if level == "" {
				level = os.Getenv("REMO_LOG")
			}
			r.logger = logging.New(level)
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&r.logLevel, "log", "", "log level (debug, info, warn)")
	cmd.AddCommand(newServerCommand(r))
	cmd.AddCommand(newConnectCommand(r))
	cmd.AddCommand(newAuthCommand(r))
	cmd.AddCommand(newKeysCommand(r))
	cmd.AddCommand(newReservationsCommand(r))
	cmd.AddCommand(newStatusCommand(r))
	cmd.AddCommand(newConnectionsCommand(r))
	cmd.AddCommand(newKillCommand(r))
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("remo version", Version)
		},
	})
	return cmd
}

func (r *rootCommand) Context() context.Context {
	return r.ctx
}

func (r *rootCommand) Logger() zerolog.Logger {
	return r.logger
}
