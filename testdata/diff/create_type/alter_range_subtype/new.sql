-- PostgreSQL has no ALTER TYPE for a range's definition: a changed subtype
-- drops and recreates the type.
CREATE TYPE span AS RANGE (subtype = bigint);
