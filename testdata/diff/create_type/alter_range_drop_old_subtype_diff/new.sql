-- The old definition's subtype_diff function goes away in the same plan: the
-- old range must be dropped before the function, and the new definition
-- created afterwards.
CREATE TYPE span AS RANGE (subtype = bigint);
