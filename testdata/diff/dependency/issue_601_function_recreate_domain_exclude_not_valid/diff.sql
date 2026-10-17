ALTER DOMAIN quantity DROP DEFAULT;

ALTER DOMAIN quantity DROP CONSTRAINT IF EXISTS quantity_max;

ALTER TABLE slots DROP CONSTRAINT IF EXISTS slots_code_excl;

ALTER TABLE slots DROP CONSTRAINT IF EXISTS slots_id_check;

DROP FUNCTION IF EXISTS default_qty();

CREATE OR REPLACE FUNCTION default_qty()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 5
$$;

DROP FUNCTION IF EXISTS max_qty();

CREATE OR REPLACE FUNCTION max_qty()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 100
$$;

DROP FUNCTION IF EXISTS normalize_code(text);

CREATE OR REPLACE FUNCTION normalize_code(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT lower(input)
$$;

ALTER DOMAIN quantity SET DEFAULT default_qty();

ALTER DOMAIN quantity ADD CONSTRAINT quantity_max CHECK (VALUE <= max_qty());

ALTER TABLE slots
ADD CONSTRAINT slots_id_check CHECK (id <= max_qty()) NOT VALID;

ALTER TABLE slots
ADD CONSTRAINT slots_code_excl EXCLUDE USING btree (normalize_code(code) WITH =);
