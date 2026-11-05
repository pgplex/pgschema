CREATE TABLE entries (
    id integer PRIMARY KEY,
    a integer,
    b integer
);

CREATE INDEX entries_sort_idx ON entries (b);
