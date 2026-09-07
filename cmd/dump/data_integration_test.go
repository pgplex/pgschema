package dump

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/testutil"
)

// TestDumpConfigData dumps config tables listed in pgschema.toml to
// CSV files plus \copy directives, then plans the dump against the same
// database and expects no changes: the round trip must be lossless.
func TestDumpConfigData(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	setup := `
		CREATE TABLE plan_tier (
			id text PRIMARY KEY,
			name text NOT NULL,
			seat_limit integer,
			note text
		);
		CREATE TABLE feature_flag (
			key text NOT NULL,
			tier text NOT NULL REFERENCES plan_tier(id),
			description text,
			enabled boolean NOT NULL DEFAULT false,
			PRIMARY KEY (key, tier)
		);
		CREATE TABLE users (
			id serial PRIMARY KEY,
			email text NOT NULL
		);
		INSERT INTO plan_tier (id, name, seat_limit, note) VALUES
			('team', 'Team', 25, 'quote '' and, comma'),
			('enterprise', 'Enterprise', NULL, ''),
			('free', 'Free', 3, E'multi\nline');
		INSERT INTO feature_flag (key, tier, description, enabled) VALUES
			('sso', 'enterprise', 'Single sign-on', true),
			('audit_log', 'team', NULL, false);
		INSERT INTO users (email) VALUES ('a@example.com');
	`
	if _, err := conn.ExecContext(ctx, setup); err != nil {
		t.Fatalf("Failed to set up schema: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pgschema.toml"), []byte("[data]\ntables = [\"plan_tier\", \"feature_*\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	config := &DumpConfig{
		Host: host, Port: port, DB: dbname, User: user, Password: password,
		Schema: "public", ConfigDir: dir,
	}

	// Without --file the CSV files have nowhere to go.
	if _, err := ExecuteDump(config); err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error for stdout dump with config tables, got %v", err)
	}

	// Multi-file dump
	mainFile := filepath.Join(dir, "main.sql")
	config.MultiFile = true
	config.File = mainFile
	if _, err := ExecuteDump(config); err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	mainContent := readFile(t, mainFile)
	wantDirectives := []string{
		`\copy feature_flag (key, tier, description, enabled) FROM 'data/feature_flag.csv' WITH (FORMAT csv, HEADER)`,
		`\copy plan_tier (id, name, seat_limit, note) FROM 'data/plan_tier.csv' WITH (FORMAT csv, HEADER)`,
	}
	for _, d := range wantDirectives {
		if !strings.Contains(mainContent, d) {
			t.Errorf("main file missing directive %q:\n%s", d, mainContent)
		}
	}
	if strings.Contains(mainContent, "users.csv") {
		t.Errorf("unlisted table users must not be dumped as data:\n%s", mainContent)
	}
	if strings.Contains(mainContent, "INSERT INTO") {
		t.Errorf("dump must not contain INSERT statements:\n%s", mainContent)
	}
	// Directives come after every include, parents before children.
	lastInclude := strings.LastIndex(mainContent, `\i `)
	firstDirective := strings.Index(mainContent, `\copy `)
	if lastInclude > firstDirective {
		t.Errorf("directives must follow all includes:\n%s", mainContent)
	}
	if strings.Index(mainContent, wantDirectives[1]) > strings.Index(mainContent, wantDirectives[0]) {
		t.Errorf("plan_tier rows must load before feature_flag rows that reference them:\n%s", mainContent)
	}
	if !strings.Contains(mainContent, "-- Name: plan_tier; Type: TABLE DATA; Schema: -; Owner: -") {
		t.Errorf("missing TABLE DATA header:\n%s", mainContent)
	}

	wantPlanTier := "id,name,seat_limit,note\n" +
		"enterprise,Enterprise,,\"\"\n" +
		"free,Free,3,\"multi\nline\"\n" +
		"team,Team,25,\"quote ' and, comma\"\n"
	if got := readFile(t, filepath.Join(dir, "data", "plan_tier.csv")); got != wantPlanTier {
		t.Errorf("plan_tier.csv mismatch:\ngot:\n%s\nwant:\n%s", got, wantPlanTier)
	}
	wantFeatureFlag := "key,tier,description,enabled\n" +
		"audit_log,team,,false\n" +
		"sso,enterprise,Single sign-on,true\n"
	if got := readFile(t, filepath.Join(dir, "data", "feature_flag.csv")); got != wantFeatureFlag {
		t.Errorf("feature_flag.csv mismatch:\ngot:\n%s\nwant:\n%s", got, wantFeatureFlag)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "users.csv")); !os.IsNotExist(err) {
		t.Errorf("users.csv must not exist")
	}

	// Round trip: planning the dump against the source database is a no-op.
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	migrationPlan, err := planCmd.GeneratePlan(&planCmd.PlanConfig{
		Host: host, Port: port, DB: dbname, User: user, Password: password,
		Schema: "public", File: mainFile, ApplicationName: "pgschema", ConfigDir: dir,
	}, provider)
	if err != nil {
		t.Fatalf("Plan of dumped files failed: %v", err)
	}
	if migrationPlan.HasAnyChanges() {
		t.Errorf("expected no changes after round trip, got:\n%s", migrationPlan.HumanColored(false))
	}

	// Single-file dump writes the schema to --file with the directives appended.
	singleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(singleDir, "pgschema.toml"), []byte("[data]\ntables = [\"plan_tier\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	singleFile := filepath.Join(singleDir, "schema.sql")
	output, err := ExecuteDump(&DumpConfig{
		Host: host, Port: port, DB: dbname, User: user, Password: password,
		Schema: "public", File: singleFile, ConfigDir: singleDir,
	})
	if err != nil {
		t.Fatalf("Single-file dump failed: %v", err)
	}
	if output != "" {
		t.Errorf("single-file dump with --file must not return output, got:\n%s", output)
	}
	single := readFile(t, singleFile)
	if !strings.HasSuffix(strings.TrimSpace(single), wantDirectives[1]) {
		t.Errorf("single-file dump must end with the plan_tier directive:\n%s", single)
	}
	if strings.Contains(single, "feature_flag.csv") {
		t.Errorf("feature_flag is not listed in this config:\n%s", single)
	}
	if _, err := os.Stat(filepath.Join(singleDir, "data", "plan_tier.csv")); err != nil {
		t.Errorf("expected data/plan_tier.csv beside single-file output: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", path, err)
	}
	return string(b)
}
