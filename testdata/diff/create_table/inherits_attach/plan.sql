ALTER TABLE capitals
ADD CONSTRAINT cities_pop_check CHECK (population >= 0::double precision) NOT VALID;

ALTER TABLE capitals VALIDATE CONSTRAINT cities_pop_check;

ALTER TABLE capitals INHERIT cities;
