package plan

import (
	"context"
	"strings"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/logger"
)

// prependPartitionParentStubs creates stub CREATE TABLE ... PARTITION BY
// statements for cross-schema partition parents referenced by PARTITION OF
// in the desired SQL. Without these stubs the plan database rejects the
// desired-state SQL because the parent table does not exist.
func prependPartitionParentStubs(ctx context.Context, cfg *util.ConnectionConfig, targetSchema, desiredSQL string) (string, error) {
	refs := postgres.ExtractPartitionOfTargets(desiredSQL, targetSchema)
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
		if ref.Schema == targetSchema {
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
		return "", err
	}
	defer conn.Close()

	var stubs strings.Builder
	for _, ref := range toStub {
		ddl, err := ir.BuildPartitionedTableStubSQL(ctx, conn, ref.Schema, ref.Table, targetSchema)
		if err != nil {
			return "", err
		}
		if ddl == "" {
			continue
		}
		logger.Get().Debug("prepending stub for cross-schema partition parent",
			"schema", ref.Schema, "table", ref.Table)
		stubs.WriteString(ddl)
	}

	return stubs.String() + desiredSQL, nil
}
