-- A previous online rewrite of entries_sort_idx finished its concurrent build
-- but was interrupted before the cutover: the valid candidate is still there.
CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer
);

CREATE INDEX entries_sort_idx ON entries (a);

CREATE INDEX entries_sort_idx_pgschema_new ON entries (b);
