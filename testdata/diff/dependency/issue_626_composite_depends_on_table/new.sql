-- Issue #626: a composite type whose attribute is a table's row type (or an
-- array of it) must be created after that table, even though it is listed
-- first in the file.
CREATE TABLE example (
    id integer PRIMARY KEY
);

CREATE TYPE example_result AS (
    rows example[]
);

-- A function returning the composite waits for it as well.
CREATE FUNCTION collect_examples() RETURNS example_result
    LANGUAGE sql STABLE
    AS $$ SELECT ARRAY(SELECT e FROM example e) $$;

-- A procedure taking the composite as a parameter waits for it too.
CREATE PROCEDURE consume_examples(IN r example_result)
    LANGUAGE plpgsql
    AS $$ BEGIN NULL; END $$;

-- A new table named like a built-in type must not be mistaken for the type:
-- payload's attribute is pg_catalog.text, so payload needs no deferral and
-- the function and table built on it keep their usual order.
CREATE TABLE text (
    id integer
);

CREATE TYPE payload AS (
    value text
);

CREATE FUNCTION check_payload(p payload) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT p.value IS NOT NULL $$;

CREATE TABLE holder (
    p payload CHECK (check_payload(p))
);
