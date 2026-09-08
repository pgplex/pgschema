package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

// TestPlanConfigDataConsistency covers the rules that keep pgschema.toml
// and the schema files in agreement.
func TestPlanConfigDataConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE country (code text PRIMARY KEY, name text NOT NULL);
		INSERT INTO country VALUES ('US', 'United States');
		CREATE TABLE keyless (code text, name text);
		CREATE TABLE genpk (code text NOT NULL, upper_code text GENERATED ALWAYS AS (upper(code)) STORED PRIMARY KEY);
	`); err != nil {
		t.Fatalf("Failed to set up schema: %v", err)
	}

	provider := testutil.SetupPostgres(t)
	defer provider.Stop()

	const ddl = "CREATE TABLE country (code text PRIMARY KEY, name text NOT NULL);\nCREATE TABLE keyless (code text, name text);\nCREATE TABLE genpk (code text NOT NULL, upper_code text GENERATED ALWAYS AS (upper(code)) STORED PRIMARY KEY);\n"
	const directive = "\\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)\n"

	run := func(t *testing.T, schemaSQL, config, ignore string) error {
		t.Helper()
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "schema.sql"), schemaSQL)
		mustWrite(t, filepath.Join(dir, "data", "country.csv"), "code,name\nUS,United States\n")
		if config != "" {
			mustWrite(t, filepath.Join(dir, "pgschema.toml"), config)
		}
		if ignore != "" {
			mustWrite(t, filepath.Join(dir, ".pgschemaignore"), ignore)
			t.Chdir(dir)
		}
		_, err := GeneratePlan(&PlanConfig{
			Host: host, Port: port, DB: dbname, User: user, Password: password,
			Schema: "public", File: filepath.Join(dir, "schema.sql"), ApplicationName: "pgschema", ConfigDir: dir,
		}, provider)
		return err
	}

	t.Run("listed and declared", func(t *testing.T) {
		if err := run(t, ddl+directive, "[data]\ntables = [\"country\"]\n", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("listed without directive", func(t *testing.T) {
		err := run(t, ddl, "[data]\ntables = [\"country\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), `has no \copy directive`) {
			t.Fatalf("expected missing directive error, got %v", err)
		}
		if !strings.Contains(err.Error(), "pgschema dump") {
			t.Errorf("error should point at dump for bootstrapping: %v", err)
		}
	})

	t.Run("directive without listing", func(t *testing.T) {
		err := run(t, ddl+directive, "", "")
		if err == nil || !strings.Contains(err.Error(), "not listed under [data]") {
			t.Fatalf("expected unlisted table error, got %v", err)
		}
	})

	t.Run("directive for table listed by a different pattern", func(t *testing.T) {
		err := run(t, ddl+directive, "[data]\ntables = [\"ref_*\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), "not listed under [data]") {
			t.Fatalf("expected unlisted table error, got %v", err)
		}
	})

	t.Run("listed table without primary key", func(t *testing.T) {
		err := run(t, ddl+directive, "[data]\ntables = [\"country\", \"keyless\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), "no primary key") {
			t.Fatalf("expected primary key error, got %v", err)
		}
	})

	t.Run("listed table with generated primary key", func(t *testing.T) {
		err := run(t, ddl+directive, "[data]\ntables = [\"country\", \"genpk\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), "generated") {
			t.Fatalf("expected generated primary key error, got %v", err)
		}
	})

	t.Run("directive outside the managed schema", func(t *testing.T) {
		other := "\\copy other.country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)\n"
		err := run(t, ddl+other, "[data]\ntables = [\"country\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), "outside the managed schema") {
			t.Fatalf("expected schema error, got %v", err)
		}
	})

	t.Run("directive for a partition", func(t *testing.T) {
		parted := "CREATE TABLE events (code text, name text, PRIMARY KEY (code, name)) PARTITION BY LIST (code);\n" +
			"CREATE TABLE events_us PARTITION OF events FOR VALUES IN ('US');\n"
		child := "\\copy events_us (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)\n"
		err := run(t, ddl+directive+parted+child, "[data]\ntables = [\"country\", \"events_us\"]\n", "")
		if err == nil || !strings.Contains(err.Error(), "through its parent table") {
			t.Fatalf("expected partition error, got %v", err)
		}
	})

	t.Run("listed and ignored", func(t *testing.T) {
		err := run(t, ddl+directive, "[data]\ntables = [\"country\"]\n", "[tables]\npatterns = [\"country\"]\n")
		if err == nil || !strings.Contains(err.Error(), "cannot be both managed and ignored") {
			t.Fatalf("expected conflict error, got %v", err)
		}
	})
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
