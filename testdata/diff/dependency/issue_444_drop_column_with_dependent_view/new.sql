CREATE TABLE foo (
    id bigint PRIMARY KEY,
    keep_me text
);

CREATE VIEW foo_v AS SELECT id, keep_me FROM foo;

CREATE VIEW foo_v2 AS SELECT id, keep_me FROM foo_v;
