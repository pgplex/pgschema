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
