-- Functions whose return type or parameter names change (DROP + CREATE)
CREATE FUNCTION max_qty() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 100 $$;
CREATE FUNCTION normalize_code(input text) RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT lower(input) $$;
CREATE FUNCTION default_qty() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

-- EXCLUDE constraint and NOT VALID CHECK constraint calling a recreated function
CREATE TABLE slots (
    id integer PRIMARY KEY,
    code text,
    CONSTRAINT slots_code_excl EXCLUDE USING btree ((normalize_code(code)) WITH =)
);
ALTER TABLE slots ADD CONSTRAINT slots_id_check CHECK (id <= max_qty()) NOT VALID;

-- Domain default and CHECK constraint calling recreated functions
CREATE DOMAIN quantity AS integer DEFAULT default_qty() CONSTRAINT quantity_max CHECK (VALUE <= max_qty());
CREATE TABLE stock (id integer PRIMARY KEY, amount quantity);
