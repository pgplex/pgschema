ALTER TABLE capitals NO INHERIT cities;

DROP TABLE IF EXISTS cities CASCADE;

ALTER TABLE capitals DROP CONSTRAINT cities_pop_check;
