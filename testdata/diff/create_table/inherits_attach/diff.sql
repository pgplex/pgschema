ALTER TABLE ONLY capitals ADD CONSTRAINT cities_pop_check CHECK (population >= 0::double precision);

ALTER TABLE capitals INHERIT cities;
