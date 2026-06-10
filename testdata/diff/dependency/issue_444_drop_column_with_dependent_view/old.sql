CREATE TABLE foo (
    id bigint PRIMARY KEY,
    keep_me text,
    drop_me text
);

CREATE VIEW foo_v AS SELECT id, keep_me, drop_me FROM foo;

CREATE VIEW foo_v2 AS SELECT id, keep_me FROM foo_v;

CREATE MATERIALIZED VIEW foo_mv AS SELECT id, keep_me FROM foo_v2;

CREATE VIEW old_consumer AS SELECT id FROM foo_v;

GRANT SELECT ON old_consumer TO PUBLIC;

GRANT SELECT ON foo_v TO PUBLIC;

CREATE FUNCTION foo_v2_noop() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_keep INSTEAD OF INSERT ON foo_v2 FOR EACH ROW EXECUTE FUNCTION foo_v2_noop();

CREATE TRIGGER trg_drop INSTEAD OF DELETE ON foo_v2 FOR EACH ROW EXECUTE FUNCTION foo_v2_noop();
