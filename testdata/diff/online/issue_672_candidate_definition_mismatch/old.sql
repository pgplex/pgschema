-- An index happens to carry the candidate name of entries_sort_idx but does
-- not match the desired definition: it must not be adopted.
CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer,
    c integer
);

CREATE INDEX entries_sort_idx ON entries (a);

CREATE INDEX entries_sort_idx_pgschema_new ON entries (c);
