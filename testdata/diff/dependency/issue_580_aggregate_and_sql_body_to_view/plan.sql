CREATE TABLE IF NOT EXISTS t (
    id integer,
    name text,
    CONSTRAINT t_pkey PRIMARY KEY (id)
);

CREATE OR REPLACE FUNCTION touch_t()
RETURNS trigger
LANGUAGE plpgsql
VOLATILE
AS $$ BEGIN PERFORM 1 FROM v; RETURN NEW; END
$$;

CREATE OR REPLACE TRIGGER touch_trg
    BEFORE INSERT ON t
    FOR EACH ROW
    EXECUTE FUNCTION touch_t();

CREATE OR REPLACE VIEW v AS
 SELECT id,
    name
   FROM t;

CREATE OR REPLACE FUNCTION acc_with_v(
    state bigint,
    x integer
)
RETURNS bigint
LANGUAGE sql
VOLATILE
AS $$ SELECT state + x + (SELECT count(*) FROM v)
$$;

CREATE OR REPLACE FUNCTION count_v()
RETURNS bigint
LANGUAGE sql
VOLATILE
AS $$ SELECT count(*) FROM v
$$;

CREATE OR REPLACE FUNCTION v_sfunc(
    state integer,
    r v
)
RETURNS integer
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + r.id
$$;

CREATE AGGREGATE sum_v(v) (
    SFUNC = v_sfunc,
    STYPE = integer,
    INITCOND = '0'
);

CREATE AGGREGATE sum_with_v(integer) (
    SFUNC = acc_with_v,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE OR REPLACE VIEW v_total AS
 SELECT sum_v(v.*) AS total
   FROM v;

CREATE OR REPLACE FUNCTION get_total()
RETURNS SETOF v_total
LANGUAGE sql
VOLATILE
AS $$ SELECT * FROM v_total
$$;
