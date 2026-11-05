CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer,
    c integer
);

CREATE INDEX entries_sort_idx ON entries (b);

CREATE INDEX entries_lookup_idx ON entries (b);
