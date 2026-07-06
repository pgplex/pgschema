--
-- Test case for GitHub issue #501: PG18 VIRTUAL generated column support
--
-- VIRTUAL generated columns (new in PG18) were silently rewritten as DEFAULT
-- expressions, producing invalid DDL at apply time.
--

CREATE TABLE vt (
    slug text NOT NULL,
    identifier text GENERATED ALWAYS AS ('urn:sdp:catalog:' || slug) VIRTUAL
);
