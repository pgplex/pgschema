CREATE OR REPLACE FUNCTION random_id()
RETURNS bigint
LANGUAGE sql
VOLATILE
AS $$
    SELECT CAST(1000000000 + floor(random() * 9000000000) AS bigint);
$$;

CREATE TABLE IF NOT EXISTS x (
    id bigint GENERATED ALWAYS AS IDENTITY,
    public_id text DEFAULT random_id() NOT NULL,
    name text NOT NULL,
    flag boolean DEFAULT false NOT NULL,
    CONSTRAINT x_pkey PRIMARY KEY (id),
    CONSTRAINT x_name_key UNIQUE (name),
    CONSTRAINT x_public_id_key UNIQUE (public_id)
);

CREATE OR REPLACE FUNCTION x_check(
    row_x x
)
RETURNS boolean
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN row_x.flag;
END;
$$;

CREATE OR REPLACE FUNCTION x_is_flagged(
    id bigint
)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
    SELECT x.flag FROM x WHERE x.id = id;
$$;
