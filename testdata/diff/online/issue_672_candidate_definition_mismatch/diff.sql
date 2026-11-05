DROP INDEX IF EXISTS entries_sort_idx_pgschema_new;

DROP INDEX IF EXISTS entries_sort_idx;

CREATE INDEX IF NOT EXISTS entries_sort_idx ON entries (b);
