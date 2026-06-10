CREATE TABLE foo (
    id bigint PRIMARY KEY,
    keep_me text,
    drop_me text
);

CREATE VIEW foo_v AS SELECT id, keep_me, drop_me FROM foo;

CREATE VIEW foo_v2 AS SELECT id, keep_me FROM foo_v;
