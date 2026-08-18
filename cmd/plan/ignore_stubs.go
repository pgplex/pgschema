package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// prependIgnoredTableStubs prepends CREATE TABLE stubs for ignored FK targets
// referenced by desiredSQL but not defined in it. Stubs are cloned from the
// target database so plan can apply REFERENCES to unmanaged tables (issue #548).
func prependIgnoredTableStubs(ctx context.Context, cfg *util.ConnectionConfig, ignoreConfig *ir.IgnoreConfig, targetSchema, desiredSQL string) (string, error) {
	if ignoreConfig == nil {
		return desiredSQL, nil
	}

	refs := postgres.ExtractForeignKeyTargets(desiredSQL, targetSchema)
	if len(refs) == 0 {
		return desiredSQL, nil
	}

	created := make(map[string]bool)
	for _, name := range postgres.ExtractCreateTableNames(desiredSQL, targetSchema) {
		created[name.Schema+"."+name.Table] = true
	}

	var toStub []postgres.QualifiedName
	seen := make(map[string]bool)
	for _, ref := range refs {
		if !ignoreConfig.ShouldIgnoreReferencedTable(ref.Schema, ref.Table, targetSchema) {
			continue
		}
		key := ref.Schema + "." + ref.Table
		if created[key] || seen[key] {
			continue
		}
		seen[key] = true
		toStub = append(toStub, ref)
	}
	if len(toStub) == 0 {
		return desiredSQL, nil
	}

	conn, err := util.Connect(cfg)
	if err != nil {
		return "", fmt.Errorf("connect to build ignored table stubs: %w", err)
	}
	defer conn.Close()

	var stubs strings.Builder
	for _, ref := range toStub {
		ddl, err := ir.BuildTableStubSQL(ctx, conn, ref.Schema, ref.Table, targetSchema)
		if err != nil {
			return "", err
		}
		if ddl == "" {
			return "", fmt.Errorf("ignored table %s.%s is referenced by a foreign key but does not exist in the target database; add a stub CREATE TABLE to your schema file, see https://www.pgschema.com/cli/plan-db", ref.Schema, ref.Table)
		}
		logger.Get().Debug("prepending stub for ignored foreign key target",
			"schema", ref.Schema, "table", ref.Table)
		stubs.WriteString(ddl)
	}

	return stubs.String() + desiredSQL, nil
}
