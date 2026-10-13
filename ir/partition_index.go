package ir

import (
	"context"
	"database/sql"
)

// buildIndexAttachments reads catalog edges independently of managed-index
// filtering. Retained constraints identify the child indexes they will create.
func (i *Inspector) buildIndexAttachments(ctx context.Context, schema *IR, targetSchema string) (map[IndexRef][]IndexRef, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT pn.nspname, parent.relname, cn.nspname, child.relname,
       t.relname, c.conname
FROM pg_inherits inh
JOIN pg_class parent ON parent.oid = inh.inhparent
JOIN pg_namespace pn ON pn.oid = parent.relnamespace
JOIN pg_class child ON child.oid = inh.inhrelid
JOIN pg_namespace cn ON cn.oid = child.relnamespace
JOIN pg_index idx ON idx.indexrelid = child.oid
JOIN pg_class t ON t.oid = idx.indrelid
LEFT JOIN pg_constraint c ON c.conindid = child.oid
    AND c.conrelid = t.oid AND c.contype IN ('p', 'u', 'x')
WHERE parent.relkind = 'I' AND pn.nspname = $1
ORDER BY pn.nspname, parent.relname, cn.nspname, child.relname`, targetSchema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make(map[IndexRef][]IndexRef)
	for rows.Next() {
		var parent, child IndexRef
		var tableName string
		var constraintName sql.NullString
		if err := rows.Scan(&parent.Schema, &parent.Name, &child.Schema, &child.Name, &tableName, &constraintName); err != nil {
			return nil, err
		}
		attachments[parent] = append(attachments[parent], child)
		if namespace := schema.Schemas[child.Schema]; namespace != nil {
			if table := namespace.Tables[tableName]; table != nil && constraintName.Valid {
				if constraint := table.Constraints[constraintName.String]; constraint != nil {
					constraint.BackingIndex = &child
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attachments, nil
}
