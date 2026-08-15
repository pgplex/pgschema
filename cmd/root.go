package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/pgplex/pgschema/cmd/apply"
	"github.com/pgplex/pgschema/cmd/dump"
	"github.com/pgplex/pgschema/cmd/plan"
	globallogger "github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/internal/version"
	"github.com/spf13/cobra"
)

var Debug bool
var logger *slog.Logger

// Build-time variables set via ldflags
var (
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var RootCmd = &cobra.Command{
	Use:   "pgschema",
	Short: "Declarative schema migration for Postgres",
	Long: fmt.Sprintf(`Declarative schema migration for Postgres

Version: %s@%s %s %s

Commands:
  dump    Dump PostgreSQL schema
  plan    Generate migration plan
  apply   Apply schema migrations

Use "pgschema [command] --help" for more information about a command.`,
		version.App(), GitCommit, platform(), BuildDate),
	Version: version.App(),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogger()
		globallogger.SetGlobal(logger, Debug)
	},
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "Enable debug logging")
	// Print a clean, machine-parseable version line, e.g. "1.12.3"
	RootCmd.SetVersionTemplate("{{.Version}}\n")
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	RootCmd.AddCommand(dump.DumpCmd)
	RootCmd.AddCommand(plan.PlanCmd)
	RootCmd.AddCommand(apply.ApplyCmd)
}

func setupLogger() {
	level := slog.LevelInfo
	if Debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	logger = slog.New(handler)
}

// GetLogger returns the global logger instance
func GetLogger() *slog.Logger {
	if logger == nil {
		setupLogger()
	}
	return logger
}

// IsDebug returns whether debug mode is enabled
func IsDebug() bool {
	return Debug
}

// platform returns the OS/architecture combination
func platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
