DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_role') THEN
        CREATE ROLE app_role;
    END IF;
END $$;

-- Unchanged on both sides (see old.sql).
CREATE FUNCTION keep_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE keeprange AS RANGE (subtype = time, subtype_diff = keep_diff);
CREATE TYPE keeptext AS RANGE (
    subtype = text,
    subtype_opclass = text_pattern_ops,
    collation = "C",
    multirange_type_name = keepspans
);
CREATE TABLE kept (
    id integer PRIMARY KEY,
    during keeprange,
    breaks keepmultirange,
    label keeptext,
    labels keepspans
);

-- Replaced: each old definition is dropped in the drop phase (before an old
-- subtype_diff or a dropped table that goes away too) and the new one created
-- in the create phase (after a new subtype_diff it names).
CREATE TYPE span_subtype AS RANGE (subtype = bigint);
CREATE FUNCTION new_diff(x integer, y integer) RETURNS double precision
    AS 'SELECT (x - y)::double precision' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE span_gains AS RANGE (subtype = integer, subtype_diff = new_diff);
CREATE TYPE span_loses AS RANGE (subtype = bigint);
CREATE TYPE span_table AS RANGE (subtype = bigint);

-- Added: a range with only a subtype. Its multirange keeps the default name
-- and is usable as a column type without being declared; the constructor
-- functions PostgreSQL creates must not appear in the plan; GRANT USAGE
-- round-trips.
CREATE TYPE floatrange AS RANGE (subtype = double precision);
CREATE TABLE reservations (
    id integer PRIMARY KEY,
    during floatrange,
    gaps floatmultirange,
    CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&)
);
GRANT USAGE ON TYPE floatrange TO app_role;

-- Added: every option that is not a function, and the two default multirange
-- name rules ("range" inside the name becomes "multirange": pricemultirange;
-- otherwise "_multirange" is appended: span_multirange).
CREATE TYPE textrange AS RANGE (
    subtype = text,
    subtype_opclass = text_pattern_ops,
    collation = "C",
    multirange_type_name = textspans
);
CREATE TYPE pricerange AS RANGE (subtype = numeric);
CREATE TYPE span AS RANGE (subtype = integer);

-- Added: a range over a new enum, and a composite attribute and a domain
-- over a new range's multirange. Each dependent sorts before what it needs
-- by name, so the order must come from the type graph.
CREATE TYPE level AS ENUM ('low', 'medium', 'high');
CREATE TYPE level_band AS RANGE (subtype = level);
CREATE TYPE period AS RANGE (subtype = date);
CREATE TYPE booking AS (room integer, slot period);
CREATE DOMAIN availability AS period_multirange;

-- Added: a range naming a function created in the same plan as its
-- subtype_diff, so it is created after the functions phase, and everything
-- built on it waits with it: a composite with a range attribute, a domain
-- over the multirange, a table with array columns of both, and a function
-- taking the range. The function's parameter types normalize to "timestamp"
-- while the subtype prints as "timestamp without time zone"; they must
-- still match.
CREATE FUNCTION ts_diff(x timestamp, y timestamp) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE stamp_span AS RANGE (subtype = timestamp, subtype_diff = ts_diff);
CREATE TYPE shift_slot AS (room integer, during stamp_span);
CREATE DOMAIN break_set AS stamp_span_multirange;
CREATE TABLE stamps (
    id integer PRIMARY KEY,
    during stamp_span,
    breaks stamp_span_multirange,
    history stamp_span[],
    past_breaks stamp_span_multirange[]
);
CREATE FUNCTION stamp_length(r stamp_span) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (upper(r) - lower(r)))' LANGUAGE sql STRICT IMMUTABLE;
