CREATE TABLE IF NOT EXISTS base (

);

CREATE DOMAIN d AS base;

CREATE TABLE IF NOT EXISTS uses (
    dcol d
);

CREATE OR REPLACE FUNCTION uses_check(
    r uses
)
RETURNS boolean
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN r.dcol IS NOT NULL;
END;
$$;
