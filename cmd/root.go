package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/pgplex/pgschema/cmd/apply"
	"github.com/pgplex/pgschema/cmd/config"
	"github.com/pgplex/pgschema/cmd/dump"
	"github.com/pgplex/pgschema/cmd/plan"
	globallogger "github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/internal/version"
	"github.com/spf13/cobra"
)

var Debug bool
var configPath string
var envName string
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
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogger()
		globallogger.SetGlobal(logger, Debug)
		loadConfig(cmd)
	},
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "Enable debug logging")
	RootCmd.PersistentFlags().StringVar(&configPath, "config", "pgschema.toml", "Path to config file")
	RootCmd.PersistentFlags().StringVar(&envName, "env", "", "Named environment to use from config file")
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

func loadConfig(cmd *cobra.Command) {
	configExplicit := cmd.Flags().Changed("config")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if configExplicit {
			fmt.Fprintf(os.Stderr, "Error: config file not found: %s\n", configPath)
			os.Exit(1)
		}
		if envName != "" {
			fmt.Fprintf(os.Stderr, "Error: --env requires a config file, but %s not found\n", configPath)
			os.Exit(1)
		}
		config.SetResolved(nil)
		return
	}

	resolved, err := config.LoadConfig(configPath, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	config.SetResolved(resolved)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
