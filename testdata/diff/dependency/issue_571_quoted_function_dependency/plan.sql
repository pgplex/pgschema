CREATE OR REPLACE FUNCTION "MyFunc"()
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN 42;
END;
$$;

CREATE TABLE IF NOT EXISTS widgets (
    id SERIAL,
    code integer DEFAULT public."MyFunc"(),
    CONSTRAINT widgets_pkey PRIMARY KEY (id)
);
