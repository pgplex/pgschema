CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer,
    c integer
);

CREATE INDEX entries_sort_idx ON entries (b);

CREATE INDEX entries_lookup_idx ON entries (b);

CREATE MATERIALIZED VIEW entries_summary AS
SELECT id, a, b, c FROM entries;

CREATE INDEX entries_summary_idx ON entries_summary (b);
