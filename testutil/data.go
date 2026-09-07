package testutil

import (
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/include"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// LoadDataConfig reads the [data] section of pgschema.toml from dir, or nil
// when the file is absent.
func LoadDataConfig(t testing.TB, dir string) *ir.DataConfig {
	t.Helper()
	dataConfig, err := util.LoadDataConfig(dir)
	if err != nil {
		t.Fatalf("Failed to load %s: %v", util.ConfigFileName, err)
	}
	return dataConfig
}

// ParseFileToIR is like ParseSQLToIRWithSetup, but takes a schema file:
// \i includes and \copy directives are resolved relative to it, and the
// rows of tables matching dataConfig are loaded into the IR. This is the
// same path the plan command uses to build the desired state.
func ParseFileToIR(t *testing.T, embeddedPG *postgres.EmbeddedPostgres, file string, schema string, setupSQL string, dataConfig *ir.DataConfig) *ir.IR {
	t.Helper()
	sqlContent, err := include.NewProcessor(filepath.Dir(file)).ProcessFile(file)
	if err != nil {
		t.Fatalf("Failed to process %s: %v", file, err)
	}
	return buildIRFromSQL(t, embeddedPG, sqlContent, schema, setupSQL, dataConfig)
}
