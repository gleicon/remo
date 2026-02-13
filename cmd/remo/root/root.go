package root

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/gleicon/remo/internal/logging"
)

type rootCommand struct {
	ctx      context.Context
	logLevel string
	logger   zerolog.Logger
}

func NewRootCommand(ctx context.Context) *cobra.Command {
	r := &rootCommand{ctx: ctx}
	cmd := &cobra.Command{
		Use:   "remo",
		Short: "Self-hosted reverse tunnel",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if r.logger.GetLevel() != zerolog.NoLevel {
				return nil
			}
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
	return cmd
}

func (r *rootCommand) Context() context.Context {
	return r.ctx
}

func (r *rootCommand) Logger() zerolog.Logger {
	return r.logger
}
