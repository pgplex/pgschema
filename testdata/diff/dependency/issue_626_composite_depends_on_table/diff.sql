CREATE TYPE payload AS (value text);

CREATE TABLE IF NOT EXISTS example (
    id integer,
    CONSTRAINT example_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS text (
    id integer
);

CREATE OR REPLACE FUNCTION check_payload(
    p payload
)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$ SELECT p.value IS NOT NULL
$$;

CREATE TABLE IF NOT EXISTS holder (
    p payload,
    CONSTRAINT holder_p_check CHECK (check_payload(p))
);

CREATE TYPE example_result AS (rows example[]);

CREATE OR REPLACE FUNCTION collect_examples()
RETURNS example_result
LANGUAGE sql
STABLE
AS $$ SELECT ARRAY(SELECT e FROM example e)
$$;

CREATE OR REPLACE PROCEDURE consume_examples(
    IN r example_result
)
LANGUAGE plpgsql
AS $$ BEGIN NULL; END 
$$;
