CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

CREATE TABLE region (
    code text PRIMARY KEY
);

CREATE DOMAIN region_row AS region;

-- The composite, the function and the domain form a cycle (the function's
-- body names the composite, the domain's CHECK calls the function, and an
-- attribute of the composite is typed on the domain), so the attribute can
-- only be added once the domain exists.
CREATE TYPE example_input AS (
    id bigint,
    label text,
    prio priority
);

-- The parameter shares the name of the attribute added below; the domain
-- needs the function, so it must not be held back for that attribute.
CREATE FUNCTION is_positive(quantity integer) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT quantity > 0 AND NULL::example_input IS NULL $$;

CREATE DOMAIN positive_int AS integer CHECK (is_positive(VALUE));

ALTER TYPE example_input ADD ATTRIBUTE region region_row;
ALTER TYPE example_input ADD ATTRIBUTE quantity positive_int;

CREATE FUNCTION get_quantity(p example_input) RETURNS integer
    LANGUAGE sql
    RETURN (p).quantity;

CREATE FUNCTION has_region(p example_input) RETURNS boolean
    LANGUAGE sql
    RETURN (p).region IS NOT NULL;

-- A SQL wrapper around a reader is created with the reader.
CREATE FUNCTION region_present() RETURNS boolean
    LANGUAGE sql
    AS $$ SELECT has_region(NULL::example_input) $$;

-- A new table whose defaults call a reader and its wrapper: both must
-- follow the attribute and precede the table.
CREATE TABLE quantities (
    n integer DEFAULT get_quantity(NULL::example_input),
    present boolean DEFAULT region_present()
);

CREATE TYPE address AS (
    street text,
    zip integer,
    city text
);

CREATE TABLE customer (
    id integer PRIMARY KEY,
    addr address
);
