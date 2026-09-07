ALTER TABLE country DROP COLUMN id;

ALTER TABLE country
ADD CONSTRAINT country_pkey PRIMARY KEY (code);

DELETE FROM country;

INSERT INTO country (code, name) VALUES ('IL', 'Israel');

INSERT INTO country (code, name) VALUES ('US', 'United States');
