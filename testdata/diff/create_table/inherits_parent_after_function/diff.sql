CREATE OR REPLACE FUNCTION is_valid_code(
    code text
)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$ SELECT code ~ '^[A-Z]{3}$'
$$;

CREATE TABLE IF NOT EXISTS a_parent (
    code text NOT NULL,
    CONSTRAINT a_parent_code_check CHECK (is_valid_code(code))
);

CREATE TABLE IF NOT EXISTS b_child (
    detail text
) INHERITS (a_parent);
