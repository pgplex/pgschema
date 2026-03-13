CREATE OR REPLACE FUNCTION create_hello(
    p_title text
)
RETURNS void
LANGUAGE plpgsql
VOLATILE
SET search_path = ''
AS $$
BEGIN
  INSERT INTO test (title) VALUES (p_title);
END;
$$;
