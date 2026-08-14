CREATE OR REPLACE FUNCTION validate_row(val anyelement)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
  RETURN val IS NOT NULL;
END
$$;

CREATE TABLE t ();

CREATE DOMAIN d AS t
  CHECK (validate_row(VALUE));
