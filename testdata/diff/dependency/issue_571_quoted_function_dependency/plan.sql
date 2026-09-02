CREATE OR REPLACE FUNCTION "MyFunc"()
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN 42;
END;
$$;

CREATE OR REPLACE FUNCTION "ZHelper"(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT upper(input)
$$;

CREATE OR REPLACE FUNCTION "AWrapper"(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT "ZHelper"(input)
$$;

CREATE TABLE IF NOT EXISTS widgets (
    id SERIAL,
    code integer DEFAULT public."MyFunc"(),
    CONSTRAINT widgets_pkey PRIMARY KEY (id)
);
