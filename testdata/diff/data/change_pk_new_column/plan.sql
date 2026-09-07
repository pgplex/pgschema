ALTER TABLE country DROP COLUMN id;

ALTER TABLE country ADD COLUMN code uuid DEFAULT gen_random_uuid() NOT NULL;

ALTER TABLE country
ADD CONSTRAINT country_pkey PRIMARY KEY (code);

DELETE FROM country;

INSERT INTO country (code, name) VALUES ('11111111-1111-4111-8111-111111111111', 'Israel');

INSERT INTO country (code, name) VALUES ('22222222-2222-4222-8222-222222222222', 'United States');
