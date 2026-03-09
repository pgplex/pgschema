package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/fingerprint"
	"github.com/pgplex/pgschema/internal/include"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/spf13/cobra"
)

var (
	planHost     string
	planPort     int
	planDB       string
	planUser     string
	planPassword string
	planSchema   string
	planFile     string
	outputHuman  string
	outputJSON   string
	outputSQL    string
	planNoColor  bool

	// Plan database flags (optional - if not provided, uses embedded postgres)
	planDBHost     string
	planDBPort     int
	planDBDatabase string
	planDBUser     string
	planDBPassword string

	planSSLMode   string
	planDBSSLMode string
)

var PlanCmd = &cobra.Command{
	Use:          "plan",
	Short:        "Generate migration plan for a specific schema",
	Long:         "Generate a migration plan to apply a desired schema state to a target database schema. Compares the desired state (from --file) with the current state of a specific schema (specified by --schema, defaults to 'public').",
	RunE:         runPlan,
	SilenceUsage: true,
	PreRunE:      util.PreRunEWithEnvVarsAndConnection(&planDB, &planUser, &planHost, &planPort),
}

func init() {
	// Target database connection flags
	PlanCmd.Flags().StringVar(&planHost, "host", "localhost", "Database server host (env: PGHOST)")
	PlanCmd.Flags().IntVar(&planPort, "port", 5432, "Database server port (env: PGPORT)")
	PlanCmd.Flags().StringVar(&planDB, "db", "", "Database name (required) (env: PGDATABASE)")
	PlanCmd.Flags().StringVar(&planUser, "user", "", "Database user name (required) (env: PGUSER)")
	PlanCmd.Flags().StringVar(&planPassword, "password", "", "Database password (optional, can also use PGPASSWORD env var)")
	PlanCmd.Flags().StringVar(&planSchema, "schema", "public", "Schema name")

	// Desired state schema file flag
	PlanCmd.Flags().StringVar(&planFile, "file", "", "Path to desired state SQL schema file (required)")

	// Plan database connection flags (optional - for using external database instead of embedded postgres)
	PlanCmd.Flags().StringVar(&planDBHost, "plan-host", "", "Plan database host (env: PGSCHEMA_PLAN_HOST). If provided, uses external database instead of embedded postgres")
	PlanCmd.Flags().IntVar(&planDBPort, "plan-port", 5432, "Plan database port (env: PGSCHEMA_PLAN_PORT)")
	PlanCmd.Flags().StringVar(&planDBDatabase, "plan-db", "", "Plan database name (env: PGSCHEMA_PLAN_DB)")
	PlanCmd.Flags().StringVar(&planDBUser, "plan-user", "", "Plan database user (env: PGSCHEMA_PLAN_USER)")
	PlanCmd.Flags().StringVar(&planDBPassword, "plan-password", "", "Plan database password (env: PGSCHEMA_PLAN_PASSWORD)")
	PlanCmd.Flags().StringVar(&planDBSSLMode, "plan-sslmode", "prefer", "Plan database SSL mode (env: PGSCHEMA_PLAN_SSLMODE)")

	// SSL mode flag
	PlanCmd.Flags().StringVar(&planSSLMode, "sslmode", "prefer", "SSL mode for database connection (disable, allow, prefer, require, verify-ca, verify-full) (env: PGSSLMODE)")

	// Output flags
	PlanCmd.Flags().StringVar(&outputHuman, "output-human", "", "Output human-readable format to stdout or file path")
	PlanCmd.Flags().StringVar(&outputJSON, "output-json", "", "Output JSON format to stdout or file path")
	PlanCmd.Flags().StringVar(&outputSQL, "output-sql", "", "Output SQL format to stdout or file path")
	PlanCmd.Flags().BoolVar(&planNoColor, "no-color", false, "Disable colored output")

	PlanCmd.MarkFlagRequired("file")
}

func runPlan(cmd *cobra.Command, args []string) error {
	// Apply environment variables to plan database flags
	util.ApplyPlanDBEnvVars(cmd, &planDBHost, &planDBDatabase, &planDBUser, &planDBPassword, &planDBPort, &planDBSSLMode)

	// Validate plan database flags if plan-host is provided
	if err := util.ValidatePlanDBFlags(planDBHost, planDBDatabase, planDBUser); err != nil {
		return err
	}

	// Derive final password: use provided password or check environment variable
	finalPassword := planPassword
	if finalPassword == "" {
		if envPassword := os.Getenv("PGPASSWORD"); envPassword != "" {
			finalPassword = envPassword
		}
	}

	// Derive final sslmode: use flag if explicitly set, otherwise check environment variable
	finalSSLMode := planSSLMode
	if cmd == nil || !cmd.Flags().Changed("sslmode") {
		if envSSLMode := os.Getenv("PGSSLMODE"); envSSLMode != "" {
			finalSSLMode = envSSLMode
		}
	}

	// Derive final plan database password
	finalPlanPassword := planDBPassword
	if finalPlanPassword == "" {
		if envPassword := os.Getenv("PGSCHEMA_PLAN_PASSWORD"); envPassword != "" {
			finalPlanPassword = envPassword
		}
	}

	// Create plan configuration
	config := &PlanConfig{
		Host:            planHost,
		Port:            planPort,
		DB:              planDB,
		User:            planUser,
		Password:        finalPassword,
		Schema:          planSchema,
		File:            planFile,
		ApplicationName: "pgschema",
		SSLMode:         finalSSLMode,
		// Plan database configuration
		PlanDBHost:     planDBHost,
		PlanDBPort:     planDBPort,
		PlanDBDatabase: planDBDatabase,
		PlanDBUser:     planDBUser,
		PlanDBPassword: finalPlanPassword,
		PlanDBSSLMode:  planDBSSLMode,
	}

	// Create desired state provider (embedded postgres or external database)
	provider, err := CreateDesiredStateProvider(config)
	if err != nil {
		return err
	}
	defer provider.Stop()

	// Generate plan
	migrationPlan, err := GeneratePlan(config, provider)
	if err != nil {
		return err
	}

	// Determine which outputs to generate
	outputs, err := determineOutputs()
	if err != nil {
		return err
	}

	// Process each output
	for _, output := range outputs {
		if err := processOutput(migrationPlan, output, cmd); err != nil {
			return err
		}
	}

	return nil
}

// PlanConfig holds configuration for plan generation
type PlanConfig struct {
	Host            string
	Port            int
	DB              string
	User            string
	Password        string
	Schema          string
	File            string
	ApplicationName string
	// Plan database configuration (optional - for external database)
	PlanDBHost     string
	PlanDBPort     int
	PlanDBDatabase string
	PlanDBUser     string
	PlanDBPassword string
	SSLMode        string
	PlanDBSSLMode  string
}

// CreateDesiredStateProvider creates either an embedded PostgreSQL instance or connects to an external database
// for validating the desired state schema. The caller is responsible for calling Stop() on the returned provider.
func CreateDesiredStateProvider(config *PlanConfig) (postgres.DesiredStateProvider, error) {
	// Detect target database PostgreSQL version (needed for both embedded and external)
	pgVersion, err := postgres.DetectPostgresVersionFromDB(
		config.Host,
		config.Port,
		config.DB,
		config.User,
		config.Password,
		config.SSLMode,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to detect PostgreSQL version: %w", err)
	}

	// Extract major version from the target database's version string (e.g., "16.9.0" -> 16).
	// The version string format is "XX.Y.Z" where XX is the major version.
	var targetMajorVersion int
	_, err = fmt.Sscanf(string(pgVersion), "%d.", &targetMajorVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL version %s: %w", pgVersion, err)
	}

	// If plan-host is provided, use external database
	if config.PlanDBHost != "" {
		externalConfig := &postgres.ExternalDatabaseConfig{
			Host:               config.PlanDBHost,
			Port:               config.PlanDBPort,
			Database:           config.PlanDBDatabase,
			Username:           config.PlanDBUser,
			Password:           config.PlanDBPassword,
			SSLMode:            config.PlanDBSSLMode,
			TargetMajorVersion: targetMajorVersion,
		}
		return postgres.NewExternalDatabase(externalConfig)
	}

	// Otherwise, use embedded PostgreSQL
	return CreateEmbeddedPostgresForPlan(config, pgVersion)
}

// CreateEmbeddedPostgresForPlan creates a temporary embedded PostgreSQL instance
// for validating the desired state schema. The instance should be stopped by the caller.
func CreateEmbeddedPostgresForPlan(config *PlanConfig, pgVersion postgres.PostgresVersion) (*postgres.EmbeddedPostgres, error) {
	if config.User == "" {
		return nil, fmt.Errorf("target database user must not be empty when creating embedded postgres")
	}

	// Start embedded PostgreSQL with matching version.
	// Use the target database username so that role references match between
	// the desired state (embedded) and current state (target database).
	// This ensures ALTER DEFAULT PRIVILEGES FOR ROLE <user> works correctly
	// and that implicit owner roles match the target database. (issue #303)
	embeddedConfig := &postgres.EmbeddedPostgresConfig{
		Version:  pgVersion,
		Database: "pgschema_temp",
		Username: config.User,
		Password: "pgschema",
	}
	embeddedPG, err := postgres.StartEmbeddedPostgres(embeddedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded PostgreSQL: %w", err)
	}

	return embeddedPG, nil
}

// GeneratePlan generates a migration plan from configuration.
// The caller must provide a non-nil provider instance for validating the desired state schema.
// The caller is responsible for managing the provider lifecycle (creation and cleanup).
func GeneratePlan(config *PlanConfig, provider postgres.DesiredStateProvider) (*plan.Plan, error) {
	// Load ignore configuration
	ignoreConfig, err := util.LoadIgnoreFileWithStructure()
	if err != nil {
		return nil, fmt.Errorf("failed to load .pgschemaignore: %w", err)
	}

	// Process desired state file with include directives
	processor := include.NewProcessor(filepath.Dir(config.File))
	desiredState, err := processor.ProcessFile(config.File)
	if err != nil {
		return nil, fmt.Errorf("failed to process desired state schema file: %w", err)
	}

	// Get current state from target database
	currentStateIR, err := util.GetIRFromDatabase(config.Host, config.Port, config.DB, config.User, config.Password, config.SSLMode, config.Schema, config.ApplicationName, ignoreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state from database: %w", err)
	}

	// Compute fingerprint of current database state
	sourceFingerprint, err := fingerprint.ComputeFingerprint(currentStateIR, config.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to compute source fingerprint: %w", err)
	}

	ctx := context.Background()

	// Apply desired state SQL to the provider (embedded postgres or external database)
	if err := provider.ApplySchema(ctx, config.Schema, desiredState); err != nil {
		return nil, fmt.Errorf("failed to apply desired state: %w", err)
	}

	// Inspect the provider database to get desired state IR
	providerHost, providerPort, providerDB, providerUsername, providerPassword := provider.GetConnectionDetails()

	// Get the temporary schema name where desired state SQL was applied.
	// Both embedded and external database providers use temporary schemas with unique timestamps
	// (e.g., pgschema_tmp_20251030_154501_123456789) to ensure isolation and prevent conflicts.
	schemaToInspect := provider.GetSchemaName()
	if schemaToInspect == "" {
		schemaToInspect = config.Schema
	}

	// Use "prefer" for provider connections (embedded postgres or external plan database)
	// since the provider's sslmode is independent of the target database's sslmode
	providerSSLMode := "prefer"
	if config.PlanDBSSLMode != "" {
		providerSSLMode = config.PlanDBSSLMode
	}
	desiredStateIR, err := util.GetIRFromDatabase(providerHost, providerPort, providerDB, providerUsername, providerPassword, providerSSLMode, schemaToInspect, config.ApplicationName, ignoreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get desired state: %w", err)
	}

	// Normalize schema names in the IR from temporary schema to target schema.
	// At this point, the IR contains schema names like "pgschema_tmp_20251030_154501_123456789"
	// because that's where objects were created. We need to replace these with the target
	// schema name (e.g., "public") so that generated DDL references the correct schema.
	// Without this normalization, DDL would reference non-existent temporary schemas and fail.
	if schemaToInspect != config.Schema {
		normalizeSchemaNames(desiredStateIR, schemaToInspect, config.Schema)
	}

	// Generate diff (current -> desired) using IR directly
	diffs := diff.GenerateMigration(currentStateIR, desiredStateIR, config.Schema)

	// Create plan from diffs with fingerprint
	migrationPlan := plan.NewPlanWithFingerprint(diffs, sourceFingerprint)

	return migrationPlan, nil
}

// outputSpec represents a single output specification
type outputSpec struct {
	format string // "human", "json", or "sql"
	target string // "stdout" or file path
}

// determineOutputs parses the output flags and returns the list of outputs to generate
func determineOutputs() ([]outputSpec, error) {
	var outputs []outputSpec
	stdoutCount := 0

	// Check each output flag
	if outputHuman != "" {
		if outputHuman == "stdout" {
			stdoutCount++
		}
		outputs = append(outputs, outputSpec{format: "human", target: outputHuman})
	}

	if outputJSON != "" {
		if outputJSON == "stdout" {
			stdoutCount++
		}
		outputs = append(outputs, outputSpec{format: "json", target: outputJSON})
	}

	if outputSQL != "" {
		if outputSQL == "stdout" {
			stdoutCount++
		}
		outputs = append(outputs, outputSpec{format: "sql", target: outputSQL})
	}

	// Validate only one stdout
	if stdoutCount > 1 {
		return nil, fmt.Errorf("only one output format can use stdout")
	}

	// Default behavior: if no outputs specified, output human to stdout
	if len(outputs) == 0 {
		outputs = append(outputs, outputSpec{format: "human", target: "stdout"})
	}

	return outputs, nil
}

// processOutput writes the plan in the specified format to the target destination
func processOutput(migrationPlan *plan.Plan, output outputSpec, cmd *cobra.Command) error {
	var content string
	var err error

	// Generate content based on format
	switch output.format {
	case "human":
		// For human format, use colored output when writing to stdout, unless explicitly disabled
		useColor := output.target == "stdout" && !planNoColor
		content = migrationPlan.HumanColored(useColor)
	case "json":
		// Check if debug flag is set on the root command
		debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
		content, err = migrationPlan.ToJSONWithDebug(debug)
		if err != nil {
			return fmt.Errorf("failed to generate JSON output: %w", err)
		}
		content += "\n"
	case "sql":
		content = migrationPlan.ToSQL(plan.SQLFormatRaw)
	default:
		return fmt.Errorf("unknown output format: %s", output.format)
	}

	// Write to target
	if output.target == "stdout" {
		fmt.Print(content)
	} else {
		// Write to file
		if err := os.WriteFile(output.target, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s output to %s: %w", output.format, output.target, err)
		}
	}

	return nil
}

// normalizeSchemaNames replaces all occurrences of fromSchema with toSchema in the IR.
//
// Context:
// During plan generation, desired state SQL is applied to a temporary schema with a unique
// timestamped name (e.g., pgschema_tmp_20251030_154501_123456789). This temporary schema
// ensures isolation and prevents conflicts when running concurrent plan operations or when
// using an external database for plan validation.
//
// When the database is inspected after applying the SQL, the IR will contain schema names
// matching the temporary schema. However, the generated DDL needs to reference the target
// schema (e.g., "public") where the migration will actually be applied.
//
// This function performs a comprehensive schema name replacement across all IR objects:
// - Tables, views, functions, procedures, types, sequences, aggregates
// - Constraints (including foreign key referenced schemas)
// - Indexes, triggers, policies
// - Dependencies, cross-references, and LIKE clauses
// - Aggregate function schemas (TransitionFunctionSchema, FinalFunctionSchema)
//
// Note: Aggregates are normalized for future-proofing even though the diff package
// does not currently support aggregate migrations.
//
// Without this normalization, generated DDL would reference non-existent temporary schemas
// and fail when applied to the target database.
func normalizeSchemaNames(irData *ir.IR, fromSchema, toSchema string) {
	replaceString := newSchemaStringReplacer(fromSchema, toSchema)
	// stripQualifiers removes same-schema function/type qualifiers from expressions.
	// After replaceString converts temp schema references to toSchema, expressions may
	// contain "toSchema.func_name(" or "::toSchema.type" which are redundant same-schema
	// qualifiers. The initial normalizeIR (run by the inspector) couldn't strip these
	// because it ran with the temp schema name, not the target schema. See issue #283.
	stripQualifiers := newSameSchemaQualifierStripper(toSchema)

	// Normalize schema names in Schemas map
	if schema, exists := irData.Schemas[fromSchema]; exists {
		delete(irData.Schemas, fromSchema)
		schema.Name = toSchema
		irData.Schemas[toSchema] = schema

		// Normalize schema names in all objects within this schema
		// Tables
		for _, table := range schema.Tables {
			table.Schema = toSchema

			// Normalize constraint schemas
			for _, constraint := range table.Constraints {
				// Normalize the constraint's own schema field
				if constraint.Schema == fromSchema {
					constraint.Schema = toSchema
				}
				// Normalize referenced schema in foreign key constraints
				if constraint.ReferencedSchema == fromSchema {
					constraint.ReferencedSchema = toSchema
				}
				constraint.CheckClause = stripQualifiers(replaceString(constraint.CheckClause))
			}

			// Normalize schema references in table dependencies
			for i := range table.Dependencies {
				if table.Dependencies[i].Schema == fromSchema {
					table.Dependencies[i].Schema = toSchema
				}
			}

			// Normalize schema references in LIKE clauses
			for i := range table.LikeClauses {
				if table.LikeClauses[i].SourceSchema == fromSchema {
					table.LikeClauses[i].SourceSchema = toSchema
				}
			}

			// Normalize column data types and expressions
			for _, column := range table.Columns {
				column.DataType = replaceString(column.DataType)
				if column.DefaultValue != nil {
					*column.DefaultValue = stripQualifiers(replaceString(*column.DefaultValue))
				}
				if column.GeneratedExpr != nil {
					*column.GeneratedExpr = stripQualifiers(replaceString(*column.GeneratedExpr))
				}
			}

			// Normalize schema names in indexes
			for _, index := range table.Indexes {
				if index.Schema == fromSchema {
					index.Schema = toSchema
				}
				index.Where = replaceString(index.Where)
			}

			// Normalize schema names in triggers
			for _, trigger := range table.Triggers {
				if trigger.Schema == fromSchema {
					trigger.Schema = toSchema
				}
				trigger.Function = replaceString(trigger.Function)
				trigger.Condition = stripQualifiers(replaceString(trigger.Condition))
			}

			// Normalize schema names in RLS policies
			for _, policy := range table.Policies {
				if policy.Schema == fromSchema {
					policy.Schema = toSchema
				}
				policy.Using = stripQualifiers(replaceString(policy.Using))
				policy.WithCheck = stripQualifiers(replaceString(policy.WithCheck))
			}
		}

		// Views
		for _, view := range schema.Views {
			view.Schema = toSchema
			view.Definition = replaceString(view.Definition)

			// Normalize schema names in materialized view indexes
			for _, index := range view.Indexes {
				if index.Schema == fromSchema {
					index.Schema = toSchema
				}
				index.Where = replaceString(index.Where)
			}

			// Normalize schema names in view triggers (e.g., INSTEAD OF triggers)
			for _, trigger := range view.Triggers {
				if trigger.Schema == fromSchema {
					trigger.Schema = toSchema
				}
				trigger.Function = stripQualifiers(replaceString(trigger.Function))
				trigger.Condition = stripQualifiers(replaceString(trigger.Condition))
			}
		}

		// Functions
		for _, fn := range schema.Functions {
			fn.Schema = toSchema
			fn.ReturnType = replaceString(fn.ReturnType)
			fn.Definition = replaceString(fn.Definition)
			fn.SearchPath = replaceString(fn.SearchPath)
			for _, param := range fn.Parameters {
				param.DataType = replaceString(param.DataType)
			}
			// Normalize function dependencies for topological sorting
			for i := range fn.Dependencies {
				fn.Dependencies[i] = replaceString(fn.Dependencies[i])
			}
		}

		// Procedures
		for _, proc := range schema.Procedures {
			proc.Schema = toSchema
			proc.Definition = replaceString(proc.Definition)
			for _, param := range proc.Parameters {
				param.DataType = replaceString(param.DataType)
			}
		}

		// Types
		for _, typ := range schema.Types {
			typ.Schema = toSchema
			typ.BaseType = replaceString(typ.BaseType)
			typ.Default = replaceString(typ.Default)
			for _, col := range typ.Columns {
				col.DataType = replaceString(col.DataType)
			}
			for _, constraint := range typ.Constraints {
				constraint.Definition = replaceString(constraint.Definition)
			}
		}

		// Sequences
		for _, seq := range schema.Sequences {
			seq.Schema = toSchema
			seq.DataType = replaceString(seq.DataType)
			seq.OwnedByTable = replaceString(seq.OwnedByTable)
		}

		// Aggregates
		for _, agg := range schema.Aggregates {
			agg.Schema = toSchema
			agg.ReturnType = replaceString(agg.ReturnType)
			agg.TransitionFunction = replaceString(agg.TransitionFunction)
			if agg.TransitionFunctionSchema == fromSchema {
				agg.TransitionFunctionSchema = toSchema
			}
			agg.StateType = replaceString(agg.StateType)
			agg.InitialCondition = replaceString(agg.InitialCondition)
			agg.FinalFunction = replaceString(agg.FinalFunction)
			if agg.FinalFunctionSchema == fromSchema {
				agg.FinalFunctionSchema = toSchema
			}
		}
	}
}

// newSchemaStringReplacer creates a string replacement function for normalizing schema names.
// It handles four replacement patterns in decreasing specificity to ensure correct schema
// name substitution across all SQL contexts.
//
// Context:
// During plan generation, temporary schemas are created with unique timestamped names
// (e.g., "pgschema_tmp_20251030_154501_123456789"). After inspecting the temporary schema,
// all references to this temporary schema must be replaced with the target schema name
// (e.g., "public") so that generated DDL references the correct deployment target.
//
// Replacement Patterns (in order):
//  1. `"fromSchema".` → `"toSchema".`  - Quoted schema qualifications (e.g., "pgschema_tmp_...".employees)
//  2. `fromSchema.`  → `toSchema.`     - Unquoted schema qualifications (e.g., pgschema_tmp_....employees)
//  3. `"fromSchema"` → `"toSchema"`    - Quoted schema references (e.g., in TYPE "pgschema_tmp_..."."status")
//  4. `fromSchema`   → `toSchema`      - Unquoted standalone references (e.g., in expressions)
//
// Why Replacement Order Matters:
// For general-purpose string replacement, processing more specific patterns (with dots) before
// less specific ones prevents double-replacement issues. For example, if replacing "temp" with
// "public", processing the bare word first could incorrectly transform "temp".table to "public".table
// before the quoted pattern gets a chance to match.
//
// Why This Implementation is Safe:
// For our specific use case with temporary schemas, the replacement order is inherently safe
// because temporary schema names are highly distinctive:
//
//   - Format: "pgschema_tmp_YYYYMMDD_HHMMSS_RRRRRRRR" (where R is a random suffix)
//   - The long, unique temporary name cannot be a substring of typical target schemas like "public"
//   - The timestamp + random suffix ensure no accidental matches with user data or identifiers
//   - The "_tmp_" marker prevents confusion with user-defined schemas
//
// This distinctive naming means that substring overlap issues that affect generic schema
// replacements (like "temp" → "public") cannot occur here. The order follows best practices
// for defensive programming and code clarity.
//
// Examples:
//
//	fromSchema: "pgschema_tmp_20251030_154501_123456789"
//	toSchema:   "public"
//
//	Input:  pgschema_tmp_20251030_154501_123456789.employees
//	Output: public.employees
//
//	Input:  "pgschema_tmp_20251030_154501_123456789".users
//	Output: "public".users
//
//	Input:  EXECUTE FUNCTION "pgschema_tmp_20251030_154501_123456789".update_time()
//	Output: EXECUTE FUNCTION "public".update_time()
//
//	Input:  TYPE pgschema_tmp_20251030_154501_123456789.status
//	Output: TYPE public.status
func newSchemaStringReplacer(fromSchema, toSchema string) func(string) string {
	if fromSchema == "" || toSchema == "" || fromSchema == toSchema {
		return func(s string) string { return s }
	}

	replacements := []string{
		fmt.Sprintf(`"%s".`, fromSchema), fmt.Sprintf(`"%s".`, toSchema),
		fmt.Sprintf(`%s.`, fromSchema), fmt.Sprintf(`%s.`, toSchema),
		fmt.Sprintf(`"%s"`, fromSchema), fmt.Sprintf(`"%s"`, toSchema),
		fromSchema, toSchema,
	}

	replacer := strings.NewReplacer(replacements...)
	return func(input string) string {
		if input == "" {
			return input
		}
		return replacer.Replace(input)
	}
}

// newSameSchemaQualifierStripper creates a function that strips redundant same-schema
// qualifiers from SQL expressions. After normalizeSchemaNames replaces temp schema names
// with the target schema, expressions may contain "schema.func_name(" or "::schema.type"
// where the qualifier matches the object's own schema. These are redundant and must be
// stripped to match how the target database's inspector would produce them. See issue #283.
func newSameSchemaQualifierStripper(schema string) func(string) string {
	if schema == "" {
		return func(s string) string { return s }
	}
	prefix := schema + "."
	funcPattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([a-zA-Z_][a-zA-Z0-9_]*)\(`)
	typePattern := regexp.MustCompile(`::` + regexp.QuoteMeta(prefix))
	return func(s string) string {
		if s == "" || !strings.Contains(s, prefix) {
			return s
		}
		s = funcPattern.ReplaceAllString(s, `${1}(`)
		s = typePattern.ReplaceAllString(s, "::")
		return s
	}
}

// ResetFlags resets all global flag variables to their default values for testing
func ResetFlags() {
	planHost = "localhost"
	planPort = 5432
	planDB = ""
	planUser = ""
	planPassword = ""
	planSchema = "public"
	planFile = ""
	outputHuman = ""
	outputJSON = ""
	outputSQL = ""
	planNoColor = false
	planDBHost = ""
	planDBPort = 5432
	planDBDatabase = ""
	planDBUser = ""
	planDBPassword = ""
	planSSLMode = "prefer"
	planDBSSLMode = "prefer"
}
