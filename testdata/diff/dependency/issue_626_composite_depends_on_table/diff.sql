CREATE TABLE IF NOT EXISTS example (
    id integer,
    CONSTRAINT example_pkey PRIMARY KEY (id)
);

CREATE TYPE example_result AS (rows example[]);

CREATE OR REPLACE FUNCTION collect_examples()
RETURNS example_result
LANGUAGE sql
STABLE
AS $$ SELECT ARRAY(SELECT e FROM example e)
$$;
