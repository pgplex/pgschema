package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// Follow immediate constraint parents to a retained originating constraint.
// Inherited copies are inspection metadata, even when their origin is ignored.
func retainedInheritedConstraint(index ir.InheritedConstraintIndex, tables map[string]*ir.Table, seen map[ir.ConstraintRef]bool) bool {
	ref := index.ParentConstraint
	if seen[ref] {
		return false
	}
	seen[ref] = true
	parent := tables[ref.Schema+"."+ref.Table]
	if parent == nil || parent.IsExternal {
		return false
	}
	if constraint := parent.Constraints[ref.Name]; constraint != nil {
		return constraint.Type == index.ConstraintType
	}
	for _, inherited := range parent.InheritedConstraintIndexes {
		if inherited.ConstraintName == ref.Name && inherited.Index == index.ParentIndex {
			return retainedInheritedConstraint(inherited, tables, seen)
		}
	}
	return false
}

// restoreInheritedIndexesSQL runs immediately after PARTITION OF. Resolve every
// index before renaming anything, then vacate all mismatched names before the
// final renames. ALTER INDEX retains constraint ownership and renames the owning
// constraint too. Catalog identities, never generated names, select the indexes.
func (p *partitionIndexCreationPlan) restoreInheritedIndexesSQL(table *ir.Table, targetSchema string, qualify bool) string {
	if p == nil {
		return ""
	}
	indexes := append([]ir.InheritedConstraintIndex(nil), p.inherited[table.Schema+"."+table.Name]...)
	if len(indexes) == 0 {
		return ""
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Index.Name < indexes[j].Index.Name })
	// Explicit escape literals also work with standard_conforming_strings off.
	literal := func(s string) string {
		return "E'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", "''") + "'"
	}
	relation := func(schema, name string) string {
		return literal(qualifyEntityNameMode(schema, name, targetSchema, qualify)) + "::pg_catalog.regclass"
	}
	var body strings.Builder
	body.WriteString(`DECLARE
    index_ids pg_catalog.oid[] := ARRAY[]::pg_catalog.oid[];
    desired_names pg_catalog.text[] := ARRAY[]::pg_catalog.text[];
    found_id pg_catalog.oid;
    current_name pg_catalog.text;
    namespace_name pg_catalog.text;
    namespace_id pg_catalog.oid;
    table_id pg_catalog.oid;
    temporary_name pg_catalog.text;
    position pg_catalog.int4;
    suffix pg_catalog.int4;
BEGIN
`)
	for _, index := range indexes {
		kind := "u"
		if index.ConstraintType == ir.ConstraintTypePrimaryKey {
			kind = "p"
		}
		fmt.Fprintf(&body, `    SELECT c.conindid INTO STRICT found_id
    FROM pg_catalog.pg_constraint c
    JOIN pg_catalog.pg_constraint pc ON pc.oid = c.conparentid
    JOIN pg_catalog.pg_class parent_index ON parent_index.oid = pc.conindid
    JOIN pg_catalog.pg_inherits inh ON inh.inhrelid = c.conindid AND inh.inhparent = pc.conindid
    WHERE c.conrelid = %s AND c.contype = %s
      AND pc.conrelid = %s AND pc.conname = %s
      AND parent_index.relname = %s;
    index_ids := pg_catalog.array_append(index_ids, found_id);
    desired_names := pg_catalog.array_append(desired_names, %s);
`, relation(table.Schema, table.Name), literal(kind), relation(index.ParentConstraint.Schema, index.ParentConstraint.Table), literal(index.ParentConstraint.Name), literal(index.ParentIndex.Name), literal(index.Index.Name))
	}
	body.WriteString(`    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname, n.oid, i.indrelid
        INTO STRICT current_name, namespace_name, namespace_id, table_id
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_catalog.pg_index i ON i.indexrelid = c.oid
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            suffix := 0;
            LOOP
                temporary_name := pg_catalog.format('pgschema_inherited_%s_%s', index_ids[position], suffix);
                EXIT WHEN NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_class
                    WHERE relnamespace = namespace_id AND relname = temporary_name
                ) AND NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_constraint
                    WHERE conrelid = table_id AND conname = temporary_name
                ) AND temporary_name <> ALL(desired_names);
                suffix := suffix + 1;
            END LOOP;
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, temporary_name);
        END IF;
    END LOOP;
    FOR position IN 1..pg_catalog.array_length(index_ids, 1) LOOP
        SELECT c.relname, n.nspname INTO STRICT current_name, namespace_name
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE c.oid = index_ids[position];
        IF current_name <> desired_names[position] THEN
            EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', namespace_name, current_name, desired_names[position]);
        END IF;
    END LOOP;
END
`)
	for n := 0; ; n++ {
		tag := fmt.Sprintf("$pgschema_restore_%d$", n)
		if !strings.Contains(body.String(), tag) {
			return "DO " + tag + "\n" + body.String() + tag + ";"
		}
	}
}
