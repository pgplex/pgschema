-- Two online rewrites were interrupted after their concurrent builds finished
-- but before the cutover, so both candidates are still there.
--
-- entries_sort_idx_pgschema_new matches the desired definition of
-- entries_sort_idx and is adopted. entries_lookup_idx_pgschema_new only shares
-- the candidate name of entries_lookup_idx: its definition differs, so it is
-- dropped and the index is rebuilt.
CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer,
    c integer
);

CREATE INDEX entries_sort_idx ON entries (a);

CREATE INDEX entries_sort_idx_pgschema_new ON entries (b);

CREATE INDEX entries_lookup_idx ON entries (a);

CREATE INDEX entries_lookup_idx_pgschema_new ON entries (c);

-- Materialized view replacements take the modified-index path; a finished
-- candidate there is adopted the same way.
CREATE MATERIALIZED VIEW entries_summary AS
SELECT id, a, b, c FROM entries;

CREATE INDEX entries_summary_idx ON entries_summary (a);

CREATE INDEX entries_summary_idx_pgschema_new ON entries_summary (b);
