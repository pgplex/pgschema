package dump

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/dump"
	"github.com/pgplex/pgschema/ir"
	"github.com/spf13/cobra"
)

// TestIRJSONRoundTrip verifies that an in-memory IR survives a JSON
// marshal → unmarshal → marshal cycle byte-for-byte. This is the contract
// downstream consumers of `dump --format=json` depend on.
func TestIRJSONRoundTrip(t *testing.T) {
	src := ir.NewIR()
	src.Metadata.DatabaseVersion = "17.4"

	public := src.CreateSchema("public")
	public.Owner = "postgres"
	public.Tables["users"] = &ir.Table{
		Schema: "public",
		Name:   "users",
		Type:   ir.TableTypeBase,
		Columns: []*ir.Column{
			{Name: "id", Position: 1, DataType: "integer", IsNullable: false},
			{Name: "email", Position: 2, DataType: "text", IsNullable: false},
		},
		Constraints: map[string]*ir.Constraint{},
		Indexes:     map[string]*ir.Index{},
		Triggers:    map[string]*ir.Trigger{},
		Policies:    map[string]*ir.RLSPolicy{},
	}

	firstPass, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		t.Fatalf("first marshal failed: %v", err)
	}

	var rehydrated ir.IR
	if err := json.Unmarshal(firstPass, &rehydrated); err != nil {
		t.Fatalf("unmarshal failed: %v\nbytes:\n%s", err, firstPass)
	}

	secondPass, err := json.MarshalIndent(&rehydrated, "", "  ")
	if err != nil {
		t.Fatalf("second marshal failed: %v", err)
	}

	if string(firstPass) != string(secondPass) {
		t.Errorf("round-trip not stable.\nfirst:\n%s\n\nsecond:\n%s", firstPass, secondPass)
	}
}

// TestExecuteDump_FormatValidation verifies the new --format flag's input checks.
func TestExecuteDump_FormatValidation(t *testing.T) {
	t.Run("unknown format rejected", func(t *testing.T) {
		_, err := ExecuteDump(&DumpConfig{
			Host:   "localhost",
			Port:   5432,
			DB:     "irrelevant",
			User:   "irrelevant",
			Schema: "public",
			Format: "yaml",
		})
		if err == nil || !strings.Contains(err.Error(), "unsupported --format") {
			t.Errorf("expected unsupported-format error, got: %v", err)
		}
	})

	t.Run("json + multi-file rejected", func(t *testing.T) {
		_, err := ExecuteDump(&DumpConfig{
			Host:      "localhost",
			Port:      5432,
			DB:        "irrelevant",
			User:      "irrelevant",
			Schema:    "public",
			Format:    FormatJSON,
			MultiFile: true,
			File:      "out.sql",
		})
		if err == nil || !strings.Contains(err.Error(), "--multi-file") {
			t.Errorf("expected json+multi-file incompatibility error, got: %v", err)
		}
	})
}

func TestDumpCommand(t *testing.T) {
	// Test that the command is properly configured
	if DumpCmd.Use != "dump" {
		t.Errorf("Expected Use to be 'dump', got '%s'", DumpCmd.Use)
	}

	if DumpCmd.Short == "" {
		t.Error("Expected Short description to be set")
	}

	if DumpCmd.Long == "" {
		t.Error("Expected Long description to be set")
	}

	// Test that required flags are defined
	flags := DumpCmd.Flags()
	dbFlag := flags.Lookup("db")
	if dbFlag == nil {
		t.Error("Expected --db flag to be defined")
	}
	userFlag := flags.Lookup("user")
	if userFlag == nil {
		t.Error("Expected --user flag to be defined")
	}

	// Test command validation - should fail without required flags
	cmd := &cobra.Command{}
	cmd.AddCommand(DumpCmd)

	// Reset the flag variables for clean test
	host = "localhost"
	port = 5432
	db = ""
	user = ""

	// Logger setup handled by root command

	err := DumpCmd.RunE(DumpCmd, []string{})
	if err == nil {
		t.Error("Expected command to fail without database connection, but it didn't")
	}
}

func TestDumpCommand_ErrorHandling(t *testing.T) {
	// Store original values
	originalHost := host
	originalPort := port
	originalDb := db
	originalUser := user

	defer func() {
		host = originalHost
		port = originalPort
		db = originalDb
		user = originalUser
	}()

	// Test with invalid connection parameters
	host = "localhost"
	port = 9999
	db = "nonexistent"
	user = "invalid"

	err := runDump(nil, nil)
	if err == nil {
		t.Error("Expected error with unreachable database, but got nil")
	}
}

func TestDumpCommand_PasswordPriority(t *testing.T) {
	// Store original values
	originalHost := host
	originalPort := port
	originalDb := db
	originalUser := user
	originalPassword := password

	defer func() {
		host = originalHost
		port = originalPort
		db = originalDb
		user = originalUser
		password = originalPassword
		os.Unsetenv("PGPASSWORD")
	}()

	t.Run("PasswordFromFlag", func(t *testing.T) {
		// Clear environment variable
		os.Unsetenv("PGPASSWORD")

		// Set flag values
		host = "localhost"
		port = 9999 // Use non-existent port to avoid actual connection
		db = "test"
		user = "test"
		password = "flag_password"

		// The password resolution happens in runDump when it calls:
		// finalPassword := password
		// if finalPassword == "" {
		//     if envPassword := os.Getenv("PGPASSWORD"); envPassword != "" {
		//         finalPassword = envPassword
		//     }
		// }
		// We can't easily test this without refactoring, but we can test the flag is set
		if password != "flag_password" {
			t.Errorf("Expected password flag to be 'flag_password', got '%s'", password)
		}
	})

	t.Run("PasswordFromEnvVar", func(t *testing.T) {
		// Set environment variable
		os.Setenv("PGPASSWORD", "env_password")

		// Clear flag
		password = ""

		// Set other required values
		host = "localhost"
		port = 9999
		db = "test"
		user = "test"

		// Verify environment variable is set
		envPassword := os.Getenv("PGPASSWORD")
		if envPassword != "env_password" {
			t.Errorf("Expected PGPASSWORD env var to be 'env_password', got '%s'", envPassword)
		}

		// Verify flag is empty (so env var should be used)
		if password != "" {
			t.Errorf("Expected password flag to be empty, got '%s'", password)
		}
	})

	t.Run("FlagOverridesEnvVar", func(t *testing.T) {
		// Set both environment variable and flag
		os.Setenv("PGPASSWORD", "env_password")
		password = "flag_password"

		// Set other required values
		host = "localhost"
		port = 9999
		db = "test"
		user = "test"

		// Flag should take precedence
		if password != "flag_password" {
			t.Errorf("Expected password flag to be 'flag_password' (should override env var), got '%s'", password)
		}

		// Environment variable should still be set
		envPassword := os.Getenv("PGPASSWORD")
		if envPassword != "env_password" {
			t.Errorf("Expected PGPASSWORD env var to be 'env_password', got '%s'", envPassword)
		}
	})

	t.Run("NoPasswordProvided", func(t *testing.T) {
		// Clear both flag and environment variable
		os.Unsetenv("PGPASSWORD")
		password = ""

		// Set other required values
		host = "localhost"
		port = 9999
		db = "test"
		user = "test"

		// Both should be empty
		if password != "" {
			t.Errorf("Expected password flag to be empty, got '%s'", password)
		}

		envPassword := os.Getenv("PGPASSWORD")
		if envPassword != "" {
			t.Errorf("Expected PGPASSWORD env var to be empty, got '%s'", envPassword)
		}
	})
}

func TestDumpCommand_EnvironmentVariables(t *testing.T) {
	// Store original values
	originalHost := host
	originalPort := port
	originalDb := db
	originalUser := user

	defer func() {
		host = originalHost
		port = originalPort
		db = originalDb
		user = originalUser
		// Clean up environment variables
		os.Unsetenv("PGHOST")
		os.Unsetenv("PGPORT")
		os.Unsetenv("PGDATABASE")
		os.Unsetenv("PGUSER")
		os.Unsetenv("PGAPPNAME")
	}()

	t.Run("EnvironmentVariablesAsDefaults", func(t *testing.T) {
		// Set environment variables
		os.Setenv("PGHOST", "env-host")
		os.Setenv("PGPORT", "9999")
		os.Setenv("PGDATABASE", "env-db")
		os.Setenv("PGUSER", "env-user")

		// Test that the PreRunE pattern works by testing the underlying helper functions
		// The actual PreRunE integration is tested in the util package
		if util.GetEnvWithDefault("PGHOST", "localhost") != "env-host" {
			t.Errorf("Expected PGHOST env var to be 'env-host', got '%s'", util.GetEnvWithDefault("PGHOST", "localhost"))
		}

		if util.GetEnvIntWithDefault("PGPORT", 5432) != 9999 {
			t.Errorf("Expected PGPORT env var to be 9999, got %d", util.GetEnvIntWithDefault("PGPORT", 5432))
		}

		if util.GetEnvWithDefault("PGDATABASE", "") != "env-db" {
			t.Errorf("Expected PGDATABASE env var to be 'env-db', got '%s'", util.GetEnvWithDefault("PGDATABASE", ""))
		}

		if util.GetEnvWithDefault("PGUSER", "") != "env-user" {
			t.Errorf("Expected PGUSER env var to be 'env-user', got '%s'", util.GetEnvWithDefault("PGUSER", ""))
		}
	})
}

func TestDumpCommand_PgpassFile(t *testing.T) {
	// Store original values
	originalHost := host
	originalPort := port
	originalDb := db
	originalUser := user
	originalPassword := password
	originalHome := os.Getenv("HOME")

	defer func() {
		host = originalHost
		port = originalPort
		db = originalDb
		user = originalUser
		password = originalPassword
		os.Unsetenv("PGPASSWORD")
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	// Create temporary directory for test
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Create .pgpass file with test credentials
	pgpassContent := "localhost:9999:testdb:testuser:pgpass_password\n"
	pgpassPath := tmpDir + "/.pgpass"
	err := os.WriteFile(pgpassPath, []byte(pgpassContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create .pgpass file: %v", err)
	}

	// Clear other password sources to ensure .pgpass would be used
	os.Unsetenv("PGPASSWORD")
	password = ""

	// Set connection parameters that match .pgpass entry
	host = "localhost"
	port = 9999
	db = "testdb"
	user = "testuser"

	// Test connection attempt - pgx driver will automatically use .pgpass
	// This will fail due to invalid connection, but verifies .pgpass integration
	err = runDump(nil, nil)
	if err == nil {
		t.Error("Expected error with unreachable database, but got nil")
	}
}

func TestDumpCommand_NoCommentsFlag(t *testing.T) {
	// Test that the --no-comments flag is defined
	flags := DumpCmd.Flags()
	noCommentsFlag := flags.Lookup("no-comments")
	if noCommentsFlag == nil {
		t.Error("Expected --no-comments flag to be defined")
	}

	// Verify default value is false
	if noCommentsFlag.DefValue != "false" {
		t.Errorf("Expected --no-comments default to be 'false', got '%s'", noCommentsFlag.DefValue)
	}
}

func TestNoComments_SingleFile(t *testing.T) {
	// Create test diffs
	diffs := []diff.Diff{
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.users",
			Source: &ir.Table{
				Name: "users",
			},
		},
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "COMMENT ON TABLE users IS 'User accounts';",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeComment,
			Operation: diff.DiffOperationCreate,
			Path:      "public.users",
			Source: &ir.Table{
				Name: "users",
			},
		},
	}

	t.Run("WithComments", func(t *testing.T) {
		formatter := dump.NewDumpFormatter("PostgreSQL 17.0", "public", false)
		output := formatter.FormatSingleFile(diffs)

		// Should contain dump header
		if !strings.Contains(output, "-- pgschema database dump") {
			t.Error("Expected output to contain dump header")
		}

		// Should contain object comment header
		if !strings.Contains(output, "-- Name: users; Type: TABLE") {
			t.Error("Expected output to contain object comment header")
		}

		// Should contain DDL
		if !strings.Contains(output, "CREATE TABLE users") {
			t.Error("Expected output to contain DDL")
		}

		// Should contain COMMENT ON statement (this is schema content, not commentary)
		if !strings.Contains(output, "COMMENT ON TABLE users") {
			t.Error("Expected output to contain COMMENT ON statement")
		}
	})

	t.Run("NoComments", func(t *testing.T) {
		formatter := dump.NewDumpFormatter("PostgreSQL 17.0", "public", true)
		output := formatter.FormatSingleFile(diffs)

		// Should still contain dump header (retained per design)
		if !strings.Contains(output, "-- pgschema database dump") {
			t.Error("Expected output to contain dump header even with --no-comments")
		}

		// Should NOT contain object comment header
		if strings.Contains(output, "-- Name: users; Type: TABLE") {
			t.Error("Expected output to NOT contain object comment header with --no-comments")
		}

		// Should still contain DDL
		if !strings.Contains(output, "CREATE TABLE users") {
			t.Error("Expected output to contain DDL")
		}

		// Should still contain COMMENT ON statement (this is schema content)
		if !strings.Contains(output, "COMMENT ON TABLE users") {
			t.Error("Expected output to contain COMMENT ON statement")
		}
	})
}
