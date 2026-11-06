-- A range type with only a subtype: the multirange keeps its default name
-- (floatmultirange) and is usable as a column type without being declared.
-- PostgreSQL also creates five constructor functions for the range; they must
-- not appear in the plan.
CREATE TYPE floatrange AS RANGE (subtype = double precision);

CREATE TABLE reservations (
    id integer PRIMARY KEY,
    during floatrange,
    gaps floatmultirange,
    CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&)
);
