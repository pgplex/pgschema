CREATE TABLE IF NOT EXISTS country (
    code text,
    name text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    CONSTRAINT country_pkey PRIMARY KEY (code)
);

INSERT INTO country (code, name, active) VALUES ('IL', 'Israel', true);

INSERT INTO country (code, name, active) VALUES ('US', 'United States', true);
