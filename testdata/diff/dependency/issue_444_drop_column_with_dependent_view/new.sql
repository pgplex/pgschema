CREATE TABLE foo (
    id bigint PRIMARY KEY,
    keep_me text
);

CREATE VIEW foo_v AS SELECT id, keep_me FROM foo;

CREATE VIEW foo_v2 AS SELECT id, keep_me FROM foo_v;

CREATE MATERIALIZED VIEW foo_mv AS SELECT id, keep_me FROM foo_v2;

CREATE FUNCTION foo_v2_noop() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_keep INSTEAD OF INSERT ON foo_v2 FOR EACH ROW EXECUTE FUNCTION foo_v2_noop();

COMMENT ON MATERIALIZED VIEW foo_mv IS 'snapshot over foo_v2';
