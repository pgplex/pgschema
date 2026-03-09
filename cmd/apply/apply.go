package apply

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/fingerprint"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/internal/version"
	"github.com/pgplex/pgschema/ir"
	"github.com/spf13/cobra"
)

var (
	applyHost            string
	applyPort            int
	applyDB              string
	applyUser            string
	applyPassword        string
	applySchema          string
	applyFile            string
	applyPlan            string
	applyAutoApprove     bool
	applyNoColor         bool
	applyLockTimeout     string
	applyApplicationName string

	// Plan database connection flags (optional - for using external database instead of embedded postgres)
	applyPlanDBHost     string
	applyPlanDBPort     int
	applyPlanDBDatabase string
	applyPlanDBUser     string
	applyPlanDBPassword string

	applySSLMode       string
	applyPlanDBSSLMode string
)

var ApplyCmd = &cobra.Command{
	Use:          "apply",
	Short:        "Apply migration plan to update a database schema",
	Long:         "Apply a migration plan to update a database schema. Either provide a desired state file (--file) to generate and apply a plan, or provide a pre-generated plan file (--plan) to execute directly.",
	RunE:         RunApply,
	SilenceUsage: true,
	PreRunE:      util.PreRunEWithEnvVarsAndConnectionAndApp(&applyDB, &applyUser, &applyHost, &applyPort, &applyApplicationName),
}

func init() {
	// Target database connection flags
	ApplyCmd.Flags().StringVar(&applyHost, "host", "localhost", "Database server host (env: PGHOST)")
	ApplyCmd.Flags().IntVar(&applyPort, "port", 5432, "Database server port (env: PGPORT)")
	ApplyCmd.Flags().StringVar(&applyDB, "db", "", "Database name (required) (env: PGDATABASE)")
	ApplyCmd.Flags().StringVar(&applyUser, "user", "", "Database user name (required) (env: PGUSER)")
	ApplyCmd.Flags().StringVar(&applyPassword, "password", "", "Database password (optional, can also use PGPASSWORD env var)")
	ApplyCmd.Flags().StringVar(&applySchema, "schema", "public", "Schema name")

	// Desired state schema file flag
	ApplyCmd.Flags().StringVar(&applyFile, "file", "", "Path to desired state SQL schema file")

	// Plan file flag
	ApplyCmd.Flags().StringVar(&applyPlan, "plan", "", "Path to plan JSON file")

	// Apply behavior flags
	ApplyCmd.Flags().BoolVar(&applyAutoApprove, "auto-approve", false, "Apply changes without prompting for approval")
	ApplyCmd.Flags().BoolVar(&applyNoColor, "no-color", false, "Disable colored output")
	ApplyCmd.Flags().StringVar(&applyLockTimeout, "lock-timeout", "", "Maximum time to wait for database locks (e.g., 30s, 5m, 1h)")
	ApplyCmd.Flags().StringVar(&applyApplicationName, "application-name", "pgschema", "Application name for database connection (visible in pg_stat_activity) (env: PGAPPNAME)")

	// Plan database connection flags (optional - for using external database instead of embedded postgres when using --file)
	ApplyCmd.Flags().StringVar(&applyPlanDBHost, "plan-host", "", "Plan database host (env: PGSCHEMA_PLAN_HOST). If provided, uses external database instead of embedded postgres for validating desired state schema")
	ApplyCmd.Flags().IntVar(&applyPlanDBPort, "plan-port", 5432, "Plan database port (env: PGSCHEMA_PLAN_PORT)")
	ApplyCmd.Flags().StringVar(&applyPlanDBDatabase, "plan-db", "", "Plan database name (env: PGSCHEMA_PLAN_DB)")
	ApplyCmd.Flags().StringVar(&applyPlanDBUser, "plan-user", "", "Plan database user (env: PGSCHEMA_PLAN_USER)")
	ApplyCmd.Flags().StringVar(&applyPlanDBPassword, "plan-password", "", "Plan database password (env: PGSCHEMA_PLAN_PASSWORD)")
	ApplyCmd.Flags().StringVar(&applyPlanDBSSLMode, "plan-sslmode", "prefer", "Plan database SSL mode (env: PGSCHEMA_PLAN_SSLMODE)")

	// SSL mode flag
	ApplyCmd.Flags().StringVar(&applySSLMode, "sslmode", "prefer", "SSL mode for database connection (disable, allow, prefer, require, verify-ca, verify-full) (env: PGSSLMODE)")

	// Mark file and plan as mutually exclusive
	ApplyCmd.MarkFlagsMutuallyExclusive("file", "plan")
}

// ApplyConfig holds configuration for apply execution
type ApplyConfig struct {
	Host            string
	Port            int
	DB              string
	User            string
	Password        string
	Schema          string
	File            string     // Desired state file (optional, used with embeddedPG)
	Plan            *plan.Plan // Pre-generated plan (optional, alternative to File)
	AutoApprove     bool
	NoColor         bool
	Quiet           bool // Suppress plan display and progress messages (useful for tests)
	LockTimeout     string
	ApplicationName string
	SSLMode         string
	// Plan database configuration (needed when GeneratePlan checks provider SSL mode)
	PlanDBHost    string
	PlanDBSSLMode string
}

// ApplyMigration applies a migration plan to update a database schema.
// The caller must provide either:
// - A pre-generated plan in config.Plan, OR
// - A desired state file in config.File with a non-nil provider instance
//
// If config.File is provided, provider is used to generate the plan.
// The caller is responsible for managing the provider lifecycle (creation and cleanup).
func ApplyMigration(config *ApplyConfig, provider postgres.DesiredStateProvider) error {
	var migrationPlan *plan.Plan
	var err error

	// Either use provided plan or generate from file
	if config.Plan != nil {
		migrationPlan = config.Plan
	} else if config.File != "" {
		// Generate plan from file (requires provider)
		if provider == nil {
			return fmt.Errorf("provider is required when generating plan from file")
		}

		planConfig := &planCmd.PlanConfig{
			Host:            config.Host,
			Port:            config.Port,
			DB:              config.DB,
			User:            config.User,
			Password:        config.Password,
			Schema:          config.Schema,
			File:            config.File,
			ApplicationName: config.ApplicationName,
			SSLMode:         config.SSLMode,
			PlanDBHost:      config.PlanDBHost,
			PlanDBSSLMode:   config.PlanDBSSLMode,
		}

		// Generate plan using shared logic
		migrationPlan, err = planCmd.GeneratePlan(planConfig, provider)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("either config.Plan or config.File must be provided")
	}

	// Load ignore configuration for fingerprint validation
	ignoreConfig, err := util.LoadIgnoreFileWithStructure()
	if err != nil {
		return fmt.Errorf("failed to load .pgschemaignore: %w", err)
	}

	// Validate schema fingerprint if plan has one
	if migrationPlan.SourceFingerprint != nil {
		err := validateSchemaFingerprint(migrationPlan, config.Host, config.Port, config.DB, config.User, config.Password, config.SSLMode, config.Schema, config.ApplicationName, ignoreConfig)
		if err != nil {
			return err
		}
	}

	// Check if there are any changes to apply by examining the plan diffs
	if !migrationPlan.HasAnyChanges() {
		fmt.Println("No changes to apply. Database schema is already up to date.")
		return nil
	}

	// Display the plan (unless quiet mode is enabled)
	if !config.Quiet {
		fmt.Print(migrationPlan.HumanColored(!config.NoColor))
	}

	// Prompt for approval if not auto-approved
	if !config.AutoApprove {
		fmt.Print("\nDo you want to apply these changes? (yes/no): ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" && response != "y" {
			fmt.Println("Apply cancelled.")
			return nil
		}
	}

	// Apply the changes
	if !config.Quiet {
		fmt.Println("\nApplying changes...")
	}

	// Build database connection for applying changes
	connConfig := &util.ConnectionConfig{
		Host:            config.Host,
		Port:            config.Port,
		Database:        config.DB,
		User:            config.User,
		Password:        config.Password,
		SSLMode:         config.SSLMode,
		ApplicationName: config.ApplicationName,
	}

	conn, err := util.Connect(connConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Set lock timeout before executing changes
	if config.LockTimeout != "" {
		lockTimeoutSQL := fmt.Sprintf("SET lock_timeout = '%s'", config.LockTimeout)
		_, err = util.ExecContextWithLogging(ctx, conn, lockTimeoutSQL, "set lock timeout")
		if err != nil {
			return fmt.Errorf("failed to set lock timeout: %w", err)
		}
	}

	// Set search_path to target schema for unqualified table references
	if config.Schema != "" && config.Schema != "public" {
		quotedSchema := ir.QuoteIdentifier(config.Schema)
		searchPathSQL := fmt.Sprintf("SET search_path TO %s, public", quotedSchema)
		_, err = util.ExecContextWithLogging(ctx, conn, searchPathSQL, "set search_path to target schema")
		if err != nil {
			return fmt.Errorf("failed to set search_path to target schema '%s': %w", config.Schema, err)
		}
		fmt.Printf("Set search_path to: %s, public\n", quotedSchema)
	}

	// Generate SQL statements from the plan
	sqlStatements := migrationPlan.ToSQL(plan.SQLFormatRaw)

	// Skip execution if no changes
	if strings.TrimSpace(sqlStatements) == "-- No changes detected" || strings.TrimSpace(sqlStatements) == "-- No DDL statements generated" {
		fmt.Println("No SQL statements to execute.")
		return nil
	}

	// Execute by groups with wait directive support
	for i, group := range migrationPlan.Groups {
		if !config.Quiet {
			fmt.Printf("\nExecuting group %d/%d...\n", i+1, len(migrationPlan.Groups))
		}

		err = executeGroup(ctx, conn, group, i+1, config.Quiet)
		if err != nil {
			return err
		}
	}

	if !config.Quiet {
		fmt.Println("Changes applied successfully!")
	}
	return nil
}

// RunApply executes the apply command logic. Exported for testing.
func RunApply(cmd *cobra.Command, args []string) error {
	// Validate that either --file or --plan is provided
	if applyFile == "" && applyPlan == "" {
		return fmt.Errorf("either --file or --plan must be specified")
	}

	// Derive final password: use provided password or check environment variable
	finalPassword := applyPassword
	if finalPassword == "" {
		if envPassword := os.Getenv("PGPASSWORD"); envPassword != "" {
			finalPassword = envPassword
		}
	}

	// Derive final sslmode: use flag if explicitly set, otherwise check environment variable
	finalSSLMode := applySSLMode
	if cmd == nil || !cmd.Flags().Changed("sslmode") {
		if envSSLMode := os.Getenv("PGSSLMODE"); envSSLMode != "" {
			finalSSLMode = envSSLMode
		}
	}

	// Validate sslmode
	if err := util.ValidateSSLMode(finalSSLMode); err != nil {
		return err
	}

	// Build configuration
	config := &ApplyConfig{
		Host:            applyHost,
		Port:            applyPort,
		DB:              applyDB,
		User:            applyUser,
		Password:        finalPassword,
		Schema:          applySchema,
		AutoApprove:     applyAutoApprove,
		NoColor:         applyNoColor,
		LockTimeout:     applyLockTimeout,
		ApplicationName: applyApplicationName,
		SSLMode:         finalSSLMode,
	}

	var provider postgres.DesiredStateProvider
	var err error

	// If using --plan flag, load plan from JSON file
	if applyPlan != "" {
		planData, err := os.ReadFile(applyPlan)
		if err != nil {
			return fmt.Errorf("failed to read plan file: %w", err)
		}

		migrationPlan, err := plan.FromJSON(planData)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}

		// Validate that the plan was generated by the same pgschema version
		currentVersion := version.App()
		if migrationPlan.PgschemaVersion != currentVersion {
			return fmt.Errorf("plan version mismatch: plan was generated by pgschema version %s, but current version is %s. Please regenerate the plan with the current version", migrationPlan.PgschemaVersion, currentVersion)
		}

		// Validate that the plan format version is supported (forward compatibility)
		supportedPlanVersion := version.PlanFormat()
		if migrationPlan.Version != supportedPlanVersion {
			return fmt.Errorf("unsupported plan format version: plan uses format version %s, but this pgschema version only supports format version %s. Please upgrade pgschema to apply this plan", migrationPlan.Version, supportedPlanVersion)
		}

		config.Plan = migrationPlan
	} else {
		// Using --file flag, will need desired state provider
		config.File = applyFile

		// Apply environment variables to plan database flags (only needed for File Mode)
		util.ApplyPlanDBEnvVars(cmd, &applyPlanDBHost, &applyPlanDBDatabase, &applyPlanDBUser, &applyPlanDBPassword, &applyPlanDBPort, &applyPlanDBSSLMode)

		// Validate plan database flags if plan-host is provided
		if err := util.ValidatePlanDBFlags(applyPlanDBHost, applyPlanDBDatabase, applyPlanDBUser); err != nil {
			return err
		}

		// Validate plan database sslmode if plan-host is provided
		if applyPlanDBHost != "" {
			if err := util.ValidateSSLMode(applyPlanDBSSLMode); err != nil {
				return fmt.Errorf("plan database: %w", err)
			}
		}

		// Derive final plan database password
		finalPlanPassword := applyPlanDBPassword
		if finalPlanPassword == "" {
			if envPassword := os.Getenv("PGSCHEMA_PLAN_PASSWORD"); envPassword != "" {
				finalPlanPassword = envPassword
			}
		}

		// Create desired state provider (embedded postgres or external database)
		planConfig := &planCmd.PlanConfig{
			Host:            applyHost,
			Port:            applyPort,
			DB:              applyDB,
			User:            applyUser,
			Password:        finalPassword,
			Schema:          applySchema,
			File:            applyFile,
			ApplicationName: applyApplicationName,
			SSLMode:         finalSSLMode,
			// Plan database configuration
			PlanDBHost:     applyPlanDBHost,
			PlanDBPort:     applyPlanDBPort,
			PlanDBDatabase: applyPlanDBDatabase,
			PlanDBUser:     applyPlanDBUser,
			PlanDBPassword: finalPlanPassword,
			PlanDBSSLMode:  applyPlanDBSSLMode,
		}
		provider, err = planCmd.CreateDesiredStateProvider(planConfig)
		if err != nil {
			return err
		}
		defer provider.Stop()

		// Propagate plan DB fields so ApplyMigration -> GeneratePlan knows the provider type
		config.PlanDBHost = applyPlanDBHost
		config.PlanDBSSLMode = applyPlanDBSSLMode
	}

	// Apply the migration
	return ApplyMigration(config, provider)
}

// validateSchemaFingerprint validates that the current database schema matches the expected fingerprint
func validateSchemaFingerprint(migrationPlan *plan.Plan, host string, port int, db, user, password, sslmode, schema, applicationName string, ignoreConfig *ir.IgnoreConfig) error {
	// Get current state from target database with ignore config
	// This ensures ignored objects are excluded from fingerprint calculation
	currentStateIR, err := util.GetIRFromDatabase(host, port, db, user, password, sslmode, schema, applicationName, ignoreConfig)
	if err != nil {
		return fmt.Errorf("failed to get current database state for fingerprint validation: %w", err)
	}

	// Compute current fingerprint
	currentFingerprint, err := fingerprint.ComputeFingerprint(currentStateIR, schema)
	if err != nil {
		return fmt.Errorf("failed to compute current fingerprint: %w", err)
	}

	// Compare with expected fingerprint
	if err := fingerprint.Compare(migrationPlan.SourceFingerprint, currentFingerprint); err != nil {
		return fmt.Errorf("%w\n\nTo resolve this issue:\n1. Regenerate the plan with current database state: pgschema plan ...\n2. Review the new plan to ensure it's still correct\n3. Apply the new plan: pgschema apply ...", err)
	}

	return nil
}

// executeGroup executes all steps in a group, handling directives separately from SQL statements
func executeGroup(ctx context.Context, conn *sql.DB, group plan.ExecutionGroup, groupNum int, quiet bool) error {
	// Check if this group has directives
	hasDirectives := false

	for _, step := range group.Steps {
		if step.Directive != nil {
			hasDirectives = true
			break
		}
	}

	if !hasDirectives {
		// No directives - concatenate all SQL and execute in implicit transaction
		return executeGroupConcatenated(ctx, conn, group, groupNum, quiet)
	} else {
		// Has directives - execute statements individually
		return executeGroupIndividually(ctx, conn, group, groupNum, quiet)
	}
}

// executeGroupConcatenated concatenates all SQL statements and executes them in an implicit transaction
func executeGroupConcatenated(ctx context.Context, conn *sql.DB, group plan.ExecutionGroup, groupNum int, quiet bool) error {
	var sqlStatements []string

	// Collect all SQL statements
	for _, step := range group.Steps {
		sqlStatements = append(sqlStatements, step.SQL)
	}

	// Concatenate all SQL statements
	concatenatedSQL := strings.Join(sqlStatements, ";\n") + ";"

	if !quiet {
		fmt.Printf("  Executing %d statements in implicit transaction\n", len(sqlStatements))
	}

	// Execute all statements in a single call (implicit transaction)
	_, err := util.ExecContextWithLogging(ctx, conn, concatenatedSQL, fmt.Sprintf("execute %d statements in group %d", len(sqlStatements), groupNum))
	if err != nil {
		return fmt.Errorf("failed to execute concatenated statements in group %d: %w", groupNum, err)
	}

	return nil
}

// executeGroupIndividually executes statements individually without transactions
func executeGroupIndividually(ctx context.Context, conn *sql.DB, group plan.ExecutionGroup, groupNum int, quiet bool) error {
	for stepIdx, step := range group.Steps {
		if step.Directive != nil {
			// Handle directive execution
			err := executeDirective(ctx, conn, step.Directive, step.SQL)
			if err != nil {
				return fmt.Errorf("directive failed in group %d, step %d: %w", groupNum, stepIdx+1, err)
			}
		} else {
			// Execute regular SQL statement
			if !quiet {
				fmt.Printf("  Executing: %s\n", truncateSQL(step.SQL, 80))
			}

			_, err := util.ExecContextWithLogging(ctx, conn, step.SQL, fmt.Sprintf("execute statement in group %d, step %d", groupNum, stepIdx+1))
			if err != nil {
				return fmt.Errorf("failed to execute statement in group %d, step %d: %w", groupNum, stepIdx+1, err)
			}
		}
	}
	return nil
}

// truncateSQL truncates a SQL statement for display purposes
func truncateSQL(sql string, maxLen int) string {
	// Remove extra whitespace and newlines
	cleaned := strings.ReplaceAll(strings.TrimSpace(sql), "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\t", " ")

	// Collapse multiple spaces into single spaces
	for strings.Contains(cleaned, "  ") {
		cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	}

	if len(cleaned) <= maxLen {
		return cleaned
	}

	return cleaned[:maxLen-3] + "..."
}
