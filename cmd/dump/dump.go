package dump

import (
	"context"
	"fmt"
	"os"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/dump"
	"github.com/pgplex/pgschema/ir"
	"github.com/spf13/cobra"
)

var (
	host          string
	port          int
	db            string
	user          string
	password      string
	schema        string
	multiFile     bool
	file          string
	noComments    bool
	sslmode       string
	qualifySchema bool
)

// DumpConfig holds configuration for dump execution
type DumpConfig struct {
	Host          string
	Port          int
	DB            string
	User          string
	Password      string
	Schema        string
	MultiFile     bool
	File          string
	NoComments    bool
	SSLMode       string
	QualifySchema bool
	// ConfigDir is where pgschema.toml is looked up. Empty means the current
	// directory.
	ConfigDir string
}

var DumpCmd = &cobra.Command{
	Use:          "dump",
	Short:        "Dump database schema for a specific schema",
	Long:         "Dump and output database schema information for a specific schema. Uses the --schema flag to target a particular schema (defaults to 'public').",
	RunE:         runDump,
	SilenceUsage: true,
	PreRunE:      util.PreRunEWithEnvVarsAndConnection(&db, &user, &host, &port),
}

func init() {
	DumpCmd.Flags().StringVar(&host, "host", "localhost", "Database server host (env: PGHOST)")
	DumpCmd.Flags().IntVar(&port, "port", 5432, "Database server port (env: PGPORT)")
	DumpCmd.Flags().StringVar(&db, "db", "", "Database name (required) (env: PGDATABASE)")
	DumpCmd.Flags().StringVar(&user, "user", "", "Database user name (required) (env: PGUSER)")
	DumpCmd.Flags().StringVar(&password, "password", "", "Database password (optional, can also use PGPASSWORD env var)")
	DumpCmd.Flags().StringVar(&schema, "schema", "public", "Schema name to dump (default: public)")
	DumpCmd.Flags().BoolVar(&multiFile, "multi-file", false, "Output schema to multiple files organized by object type")
	DumpCmd.Flags().StringVar(&file, "file", "", "Output file path (required when --multi-file is used)")
	DumpCmd.Flags().BoolVar(&noComments, "no-comments", false, "Do not output object comment headers")
	DumpCmd.Flags().BoolVar(&qualifySchema, "qualify-schema", false, "Always schema-qualify object identifiers and type references in the dump, even for the target schema (function/procedure parameter and return types are not yet qualified)")
	DumpCmd.Flags().StringVar(&sslmode, "sslmode", "prefer", "SSL mode for database connection (disable, allow, prefer, require, verify-ca, verify-full) (env: PGSSLMODE)")
}

// ExecuteDump executes the dump operation with the given configuration
func ExecuteDump(config *DumpConfig) (string, error) {
	// Validate flags
	if config.MultiFile && config.File == "" {
		// When --multi-file is used but no --file specified, emit warning and use single-file mode
		fmt.Fprintf(os.Stderr, "Warning: --multi-file flag requires --file to be specified. Fallback to single-file mode.\n")
		config.MultiFile = false
	}

	// Load ignore configuration
	ignoreConfig, err := util.LoadIgnoreFileWithStructure()
	if err != nil {
		return "", fmt.Errorf("failed to load .pgschemaignore: %w", err)
	}

	// Load project configuration (reference tables)
	dataConfig, err := util.LoadDataConfig(config.ConfigDir)
	if err != nil {
		return "", err
	}

	conn, err := util.Connect(&util.ConnectionConfig{
		Host: config.Host, Port: config.Port, Database: config.DB, User: config.User,
		Password: config.Password, SSLMode: config.SSLMode, ApplicationName: "pgschema",
	})
	if err != nil {
		return "", err
	}
	defer conn.Close()
	ctx := context.Background()

	// Inspect the schema. Reference tables are only marked, not loaded: their
	// rows are exported below with COPY straight into CSV files.
	schemaIR, err := ir.NewInspector(conn, ignoreConfig).WithDataConfig(dataConfig, false).BuildIR(ctx, config.Schema)
	if err != nil {
		return "", fmt.Errorf("failed to get database schema: %w", err)
	}

	// Reference tables: rows go to data/<table>.csv beside the output file and
	// a \copy directive is appended after every other object.
	dataTables := dump.DataTables(schemaIR, config.Schema)
	if len(dataTables) > 0 {
		if dbSchema, ok := schemaIR.Schemas[config.Schema]; ok {
			if err := util.ValidateDataAgainstIgnore(dataConfig, ignoreConfig, dbSchema.TableNames()); err != nil {
				return "", err
			}
		}
		if config.File == "" {
			return "", fmt.Errorf("%s lists reference tables, so --file is required: their rows are written to %s/ next to the output file", util.ConfigFileName, dump.DataDir)
		}
	}

	// Create an empty schema for comparison to generate a dump diff
	emptyIR := ir.NewIR()

	// Generate diff between empty schema and target schema (this represents a complete dump)
	diffs := diff.GenerateMigrationWithOptions(emptyIR, schemaIR, config.Schema, config.QualifySchema)

	// Create dump formatter
	formatter := dump.NewDumpFormatter(schemaIR.Metadata.DatabaseVersion, config.Schema, config.NoComments, config.QualifySchema)
	formatter.SetDataTables(dataTables)
	if err := formatter.WriteDataFiles(ctx, conn, dataTables, config.File); err != nil {
		return "", err
	}

	if config.MultiFile {
		// Multi-file mode - output to files
		err := formatter.FormatMultiFile(diffs, config.File)
		if err != nil {
			return "", fmt.Errorf("failed to create multi-file output: %w", err)
		}
		return "", nil
	}

	// Single file mode - return output as string, or write it to --file
	output := formatter.FormatSingleFile(diffs)
	if config.File != "" {
		if err := os.WriteFile(config.File, []byte(output), 0644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", config.File, err)
		}
		return "", nil
	}
	return output, nil
}

func runDump(cmd *cobra.Command, args []string) error {
	// Derive final password: use flag if provided, otherwise check environment variable
	finalPassword := password
	if finalPassword == "" {
		if envPassword := os.Getenv("PGPASSWORD"); envPassword != "" {
			finalPassword = envPassword
		}
	}

	// Derive final sslmode: use flag if explicitly set, otherwise check environment variable
	finalSSLMode := sslmode
	if cmd == nil || !cmd.Flags().Changed("sslmode") {
		if envSSLMode := os.Getenv("PGSSLMODE"); envSSLMode != "" {
			finalSSLMode = envSSLMode
		}
	}

	// Validate sslmode
	if err := util.ValidateSSLMode(finalSSLMode); err != nil {
		return err
	}

	// Create config from command-line flags
	config := &DumpConfig{
		Host:          host,
		Port:          port,
		DB:            db,
		User:          user,
		Password:      finalPassword,
		Schema:        schema,
		MultiFile:     multiFile,
		File:          file,
		NoComments:    noComments,
		SSLMode:       finalSSLMode,
		QualifySchema: qualifySchema,
	}

	// Execute dump
	output, err := ExecuteDump(config)
	if err != nil {
		return err
	}

	// Print output to stdout (only in single-file mode)
	if output != "" {
		fmt.Print(output)
	}

	return nil
}
