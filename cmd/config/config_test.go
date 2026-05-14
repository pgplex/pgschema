package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_MinimalFlat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
host = "localhost"
port = 5432
db = "myapp_dev"
user = "postgres"
schema = "public"
file = "schema.sql"
`), 0644)

	resolved, err := LoadConfig(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Host != "localhost" {
		t.Errorf("Host = %q, want %q", resolved.Host, "localhost")
	}
	if resolved.Port != 5432 {
		t.Errorf("Port = %d, want %d", resolved.Port, 5432)
	}
	if resolved.DB != "myapp_dev" {
		t.Errorf("DB = %q, want %q", resolved.DB, "myapp_dev")
	}
	if resolved.User != "postgres" {
		t.Errorf("User = %q, want %q", resolved.User, "postgres")
	}
	if resolved.Schema != "public" {
		t.Errorf("Schema = %q, want %q", resolved.Schema, "public")
	}
	if resolved.File != "schema.sql" {
		t.Errorf("File = %q, want %q", resolved.File, "schema.sql")
	}
	if resolved.Schemas != nil {
		t.Errorf("Schemas = %v, want nil", resolved.Schemas)
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
schema = "public"
file = "schema.sql"
lock-timeout = "10s"

[env.dev]
host = "localhost"
port = 5432
db = "myapp_dev"
user = "postgres"

[env.prod]
host = "prod-db.internal"
db = "myapp_prod"
user = "app_user"
lock-timeout = "60s"
`), 0644)

	t.Run("base only", func(t *testing.T) {
		resolved, err := LoadConfig(path, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Schema != "public" {
			t.Errorf("Schema = %q, want %q", resolved.Schema, "public")
		}
		if resolved.Host != "" {
			t.Errorf("Host = %q, want empty", resolved.Host)
		}
	})

	t.Run("dev env inherits base", func(t *testing.T) {
		resolved, err := LoadConfig(path, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Schema != "public" {
			t.Errorf("Schema = %q, want %q (inherited from base)", resolved.Schema, "public")
		}
		if resolved.File != "schema.sql" {
			t.Errorf("File = %q, want %q (inherited from base)", resolved.File, "schema.sql")
		}
		if resolved.Host != "localhost" {
			t.Errorf("Host = %q, want %q", resolved.Host, "localhost")
		}
		if resolved.LockTimeout != "10s" {
			t.Errorf("LockTimeout = %q, want %q (inherited from base)", resolved.LockTimeout, "10s")
		}
	})

	t.Run("prod env overrides base", func(t *testing.T) {
		resolved, err := LoadConfig(path, "prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.LockTimeout != "60s" {
			t.Errorf("LockTimeout = %q, want %q (overridden by prod)", resolved.LockTimeout, "60s")
		}
		if resolved.File != "schema.sql" {
			t.Errorf("File = %q, want %q (inherited from base)", resolved.File, "schema.sql")
		}
	})
}

func TestLoadConfig_BooleanOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
auto-approve = true

[env.safe]
auto-approve = false
`), 0644)

	resolved, err := LoadConfig(path, "safe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.AutoApprove != false {
		t.Errorf("AutoApprove = %v, want false (explicit override of base true)", resolved.AutoApprove)
	}
}

func TestLoadConfig_UnknownEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
db = "test"
`), 0644)

	_, err := LoadConfig(path, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown env, got nil")
	}
	expected := `environment "nonexistent" not found`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), expected)
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
this is not valid toml [[[
`), 0644)

	_, err := LoadConfig(path, "")
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestLoadConfig_SchemasQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pgschema.toml")
	os.WriteFile(path, []byte(`
file = "tenant.sql"

[env.tenants]
host = "localhost"
db = "myapp"
user = "postgres"

[env.tenants.schemas]
query = "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%'"
`), 0644)

	resolved, err := LoadConfig(path, "tenants")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Schemas == nil {
		t.Fatal("Schemas is nil, want non-nil")
	}
	if resolved.Schemas.Query != "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%'" {
		t.Errorf("Schemas.Query = %q, want tenant query", resolved.Schemas.Query)
	}
	if resolved.File != "tenant.sql" {
		t.Errorf("File = %q, want %q (inherited from base)", resolved.File, "tenant.sql")
	}
}
