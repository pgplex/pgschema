CREATE TYPE floatrange AS RANGE (subtype = double precision);

CREATE TABLE reservations (
    id integer PRIMARY KEY,
    during floatrange,
    gaps floatmultirange
);
