DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_role') THEN
        CREATE ROLE app_role;
    END IF;
END $$;

-- Unchanged on both sides: a range with a subtype_diff, a range with every
-- option, and a table using both and their multiranges. None of it may
-- appear in the plan.
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

-- Dropped: the table first, then the overload that takes the range, then
-- the range, then its subtype_diff function.
CREATE FUNCTION time_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE timerange AS RANGE (subtype = time, subtype_diff = time_diff);
CREATE FUNCTION time_diff(r timerange) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (upper(r) - lower(r)))' LANGUAGE sql STRICT IMMUTABLE;
CREATE TABLE shifts (
    id integer PRIMARY KEY,
    during timerange
);

-- Dropped chain: the domain over the multirange, the range, its subtype_diff,
-- the helper that function calls, and only then the enum both functions take.
CREATE TYPE severity AS ENUM ('low', 'high');
CREATE FUNCTION severity_num(s severity) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE s WHEN 'low' THEN 0 ELSE 1 END;
CREATE FUNCTION severity_diff(a severity, b severity) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN severity_num(a) - severity_num(b);
CREATE TYPE band AS RANGE (subtype = severity, subtype_diff = severity_diff);
CREATE DOMAIN band_set AS band_multirange;

-- Dropped chain through a checked domain: the range, its subtype_diff whose
-- signature uses the domain, the domain, then the domain's CHECK function.
CREATE FUNCTION is_positive(v integer) RETURNS boolean
    AS 'SELECT v > 0' LANGUAGE sql STRICT IMMUTABLE;
CREATE DOMAIN positive AS integer CHECK (is_positive(VALUE));
CREATE FUNCTION positive_diff(a positive, b positive) RETURNS double precision
    AS 'SELECT (a - b)::double precision' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE positive_span AS RANGE (subtype = positive, subtype_diff = positive_diff);

-- Dropped, with body dependencies. a_diff's SQL-standard body casts to the
-- enum z_status (a recorded dependency) and mentions z_span only in a string
-- literal (not one). z_diff's body casts to a_span (recorded). So z_span
-- drops first, then z_diff, then a_span, then a_diff, then z_status.
CREATE TYPE z_status AS ENUM ('on', 'off');
CREATE FUNCTION a_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN 'z_span' = 'z_span' AND 'on'::z_status = 'on' THEN (x - y)::double precision ELSE 0 END;
CREATE TYPE a_span AS RANGE (subtype = integer, subtype_diff = a_diff);
CREATE FUNCTION z_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN '[1,2)'::a_span @> 1 THEN (x - y)::double precision ELSE 0 END;
CREATE TYPE z_span AS RANGE (subtype = integer, subtype_diff = z_diff);

-- Replaced (PostgreSQL has no ALTER TYPE for a range's definition): one
-- changes subtype, one gains a subtype_diff created in the same plan, one
-- loses its subtype_diff dropped in the same plan, one loses the table that
-- used it.
CREATE TYPE span_subtype AS RANGE (subtype = integer);
CREATE TYPE span_gains AS RANGE (subtype = integer);
CREATE FUNCTION old_diff(x integer, y integer) RETURNS double precision
    AS 'SELECT (x - y)::double precision' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE span_loses AS RANGE (subtype = integer, subtype_diff = old_diff);
CREATE TYPE span_table AS RANGE (subtype = integer);
CREATE TABLE windows (
    id integer PRIMARY KEY,
    during span_table
);
