CREATE OR REPLACE FUNCTION "MyFunc"(
    val integer DEFAULT 0
)
RETURNS integer
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    RETURN val * 2;
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

CREATE INDEX IF NOT EXISTS widgets_active_partial_idx ON widgets (id) WHERE ("MyFunc"(code) > 0);

CREATE INDEX IF NOT EXISTS widgets_code_myfunc_idx ON widgets ("MyFunc"(code));
