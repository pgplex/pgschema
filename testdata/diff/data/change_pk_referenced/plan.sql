ALTER TABLE city DROP CONSTRAINT city_country_code_fkey;

ALTER TABLE country DROP CONSTRAINT country_code_key;

ALTER TABLE country DROP COLUMN id;

ALTER TABLE country
ADD CONSTRAINT country_pkey PRIMARY KEY (code);

ALTER TABLE city
ADD CONSTRAINT city_country_code_fkey FOREIGN KEY (country_code) REFERENCES country (code) NOT VALID;

ALTER TABLE city VALIDATE CONSTRAINT city_country_code_fkey;

UPDATE country SET name = 'State of Israel' WHERE code = 'IL';
