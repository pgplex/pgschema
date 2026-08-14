CREATE TABLE IF NOT EXISTS t (

);

CREATE OR REPLACE FUNCTION validate_row(
    val anyelement
)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
  RETURN val IS NOT NULL;
END
$$;

CREATE DOMAIN d AS t
  CONSTRAINT d_check CHECK (validate_row(VALUE));
