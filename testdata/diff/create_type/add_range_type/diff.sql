CREATE TYPE floatrange AS RANGE (
    subtype = double precision
);

CREATE TABLE IF NOT EXISTS reservations (
    id integer,
    during floatrange,
    gaps floatmultirange,
    CONSTRAINT reservations_pkey PRIMARY KEY (id),
    CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&)
);
