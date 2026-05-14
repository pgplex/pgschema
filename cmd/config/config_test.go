package config

import (
	"os"
	"path/filepath"
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
