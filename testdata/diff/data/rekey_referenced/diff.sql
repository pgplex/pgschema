INSERT INTO country (code, name) VALUES ('USA', 'United States');

UPDATE city SET country_code = 'USA' WHERE name = 'Austin';

DELETE FROM country WHERE code = 'US';
