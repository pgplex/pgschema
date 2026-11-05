DROP INDEX IF EXISTS entries_lookup_idx_pgschema_new;

DROP INDEX IF EXISTS entries_lookup_idx;

CREATE INDEX IF NOT EXISTS entries_lookup_idx ON entries (b);

DROP INDEX IF EXISTS entries_sort_idx;

ALTER INDEX entries_sort_idx_pgschema_new RENAME TO entries_sort_idx;

DROP INDEX IF EXISTS entries_summary_idx;

ALTER INDEX entries_summary_idx_pgschema_new RENAME TO entries_summary_idx;
