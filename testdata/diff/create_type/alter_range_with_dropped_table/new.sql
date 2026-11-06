-- The table using the old definition is dropped in the same plan, so the
-- replacement must come after the drop phase or DROP TYPE ... RESTRICT fails.
CREATE TYPE span AS RANGE (subtype = bigint);
