DROP INDEX IF EXISTS entries_sort_idx;

ALTER INDEX entries_sort_idx_pgschema_new RENAME TO entries_sort_idx;
