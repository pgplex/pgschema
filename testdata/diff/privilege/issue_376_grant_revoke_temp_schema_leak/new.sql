DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'appname_apiuser') THEN
        CREATE ROLE appname_apiuser;
    END IF;
END $$;

CREATE TYPE my_input AS (
    id uuid
);

CREATE FUNCTION f_test(p_items my_input[])
RETURNS integer
LANGUAGE sql
AS $$
  SELECT COALESCE(array_length(p_items, 1), 0);
$$;

REVOKE ALL ON FUNCTION f_test(my_input[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION f_test(my_input[]) TO appname_apiuser;
