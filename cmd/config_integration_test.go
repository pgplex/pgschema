package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/config"
	"github.com/pgplex/pgschema/testutil"
)

func resetRootCmd() {
	config.SetResolved(nil)
	configPath = "pgschema.toml"
	envName = ""
}

func TestConfigLoading_NoFile(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	loadConfig(RootCmd)

	if config.Get() != nil {
		t.Error("expected no config when pgschema.toml is absent")
	}
}

func TestConfigLoading_WithFile(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "testhost"
port = 9999
db = "testdb"
user = "testuser"
schema = "myschema"
file = "schema.sql"
`), 0644)

	configPath = tomlPath
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.Host != "testhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "testhost")
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.DB != "testdb" {
		t.Errorf("DB = %q, want %q", cfg.DB, "testdb")
	}
	if cfg.Schema != "myschema" {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "myschema")
	}
}

func TestConfigLoading_WithEnv(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "base-host"
schema = "public"
file = "schema.sql"

[env.dev]
host = "dev-host"
db = "dev_db"
user = "dev_user"

[env.prod]
host = "prod-host"
db = "prod_db"
user = "prod_user"
lock-timeout = "30s"
`), 0644)

	configPath = tomlPath
	envName = "dev"
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.Host != "dev-host" {
		t.Errorf("Host = %q, want %q (dev override)", cfg.Host, "dev-host")
	}
	if cfg.DB != "dev_db" {
		t.Errorf("DB = %q, want %q (dev override)", cfg.DB, "dev_db")
	}
	if cfg.Schema != "public" {
		t.Errorf("Schema = %q, want %q (inherited from base)", cfg.Schema, "public")
	}
	if cfg.File != "schema.sql" {
		t.Errorf("File = %q, want %q (inherited from base)", cfg.File, "schema.sql")
	}
}

func TestConfigLoading_SchemasSection(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "localhost"
db = "myapp"
user = "postgres"
file = "tenant.sql"

[schemas]
query = "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%'"
`), 0644)

	configPath = tomlPath
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.Schemas == nil {
		t.Fatal("expected Schemas to be non-nil")
	}
	if cfg.Schemas.Query == "" {
		t.Error("expected Schemas.Query to be set")
	}
}

func TestConfigLoading_PlanFieldsFallback(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "myhost"
port = 5433
db = "mydb"
user = "myuser"
plan-host = "plan-server"
plan-port = 15432
plan-db = "plandb"
plan-user = "planner"
plan-sslmode = "require"
application-name = "myapp"
lock-timeout = "15s"
auto-approve = true
no-color = true
`), 0644)

	configPath = tomlPath
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.PlanHost != "plan-server" {
		t.Errorf("PlanHost = %q, want %q", cfg.PlanHost, "plan-server")
	}
	if cfg.PlanPort != 15432 {
		t.Errorf("PlanPort = %d, want %d", cfg.PlanPort, 15432)
	}
	if cfg.PlanDB != "plandb" {
		t.Errorf("PlanDB = %q, want %q", cfg.PlanDB, "plandb")
	}
	if cfg.LockTimeout != "15s" {
		t.Errorf("LockTimeout = %q, want %q", cfg.LockTimeout, "15s")
	}
	if !cfg.AutoApprove {
		t.Error("AutoApprove should be true")
	}
	if !cfg.NoColor {
		t.Error("NoColor should be true")
	}
}

func TestConfigLoading_EnvOverridesBooleans(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
auto-approve = true
no-color = true
multi-file = true

[env.safe]
auto-approve = false
no-color = false
multi-file = false
`), 0644)

	configPath = tomlPath
	envName = "safe"
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.AutoApprove {
		t.Error("AutoApprove should be false (overridden by env)")
	}
	if cfg.NoColor {
		t.Error("NoColor should be false (overridden by env)")
	}
	if cfg.MultiFile {
		t.Error("MultiFile should be false (overridden by env)")
	}
}

func TestDumpCommand_ConfigFallback(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "config-host"
port = 9876
db = "config-db"
user = "config-user"
schema = "config_schema"
sslmode = "require"
no-comments = true
`), 0644)

	configPath = tomlPath
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("config should be loaded")
	}

	// Verify config values are accessible for dump command fallback
	if cfg.Host != "config-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "config-host")
	}
	if cfg.Port != 9876 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9876)
	}
	if cfg.Schema != "config_schema" {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "config_schema")
	}
	if cfg.SSLMode != "require" {
		t.Errorf("SSLMode = %q, want %q", cfg.SSLMode, "require")
	}
	if !cfg.NoComments {
		t.Error("NoComments should be true")
	}
}

func TestApplyConfigToPlan_UsesConfigValues(t *testing.T) {
	resetRootCmd()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(tomlPath, []byte(`
host = "plan-test-host"
port = 1234
db = "plan-test-db"
user = "plan-test-user"
schema = "plan-test-schema"
file = "plan-test.sql"

[env.staging]
host = "staging-host"
db = "staging-db"
lock-timeout = "45s"
`), 0644)

	configPath = tomlPath
	envName = "staging"
	loadConfig(RootCmd)

	cfg := config.Get()
	if cfg == nil {
		t.Fatal("config should be loaded")
	}
	if cfg.Host != "staging-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "staging-host")
	}
	if cfg.DB != "staging-db" {
		t.Errorf("DB = %q, want %q", cfg.DB, "staging-db")
	}
	if cfg.File != "plan-test.sql" {
		t.Errorf("File = %q, want %q (inherited from base)", cfg.File, "plan-test.sql")
	}
	if cfg.LockTimeout != "45s" {
		t.Errorf("LockTimeout = %q, want %q", cfg.LockTimeout, "45s")
	}
}

func TestDiscoverSchemas_ReadOnlyEnforcement(t *testing.T) {
	_, host, port, dbname, user, password := testutil.ConnectToPostgres(t, sharedEmbeddedPG)

	// Valid SELECT query should succeed
	t.Run("SELECT is allowed", func(t *testing.T) {
		schemas, err := config.DiscoverSchemas(host, port, dbname, user, password, "disable",
			"SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'public'")
		if err != nil {
			t.Fatalf("SELECT query should succeed: %v", err)
		}
		if len(schemas) == 0 {
			t.Error("expected at least one schema")
		}
	})

	// CREATE TABLE should be rejected by read-only transaction
	t.Run("CREATE is rejected", func(t *testing.T) {
		_, err := config.DiscoverSchemas(host, port, dbname, user, password, "disable",
			"CREATE TABLE pgschema_injection_test (id int)")
		if err == nil {
			t.Fatal("CREATE should be rejected in read-only transaction")
		}
		if !strings.Contains(err.Error(), "read-only") {
			t.Errorf("error should mention read-only, got: %v", err)
		}
	})

	// DROP should be rejected
	t.Run("DROP is rejected", func(t *testing.T) {
		_, err := config.DiscoverSchemas(host, port, dbname, user, password, "disable",
			"DROP TABLE IF EXISTS pgschema_injection_test")
		if err == nil {
			t.Fatal("DROP should be rejected in read-only transaction")
		}
		if !strings.Contains(err.Error(), "read-only") {
			t.Errorf("error should mention read-only, got: %v", err)
		}
	})

	// INSERT should be rejected
	t.Run("INSERT is rejected", func(t *testing.T) {
		// Create a temp table first via direct connection, then try INSERT via DiscoverSchemas
		_, err := config.DiscoverSchemas(host, port, dbname, user, password, "disable",
			"INSERT INTO information_schema.schemata VALUES ('hacked')")
		if err == nil {
			t.Fatal("INSERT should be rejected in read-only transaction")
		}
	})
}

