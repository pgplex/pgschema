ALTER TABLE cities ADD COLUMN country text DEFAULT 'US' NOT NULL;

ALTER TABLE cities
ADD CONSTRAINT cities_pop_check CHECK (population >= 0::double precision);

CREATE INDEX IF NOT EXISTS capitals_country_idx ON capitals (country);
