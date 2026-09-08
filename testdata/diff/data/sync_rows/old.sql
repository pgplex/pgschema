CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL,
    active boolean NOT NULL DEFAULT true
);

INSERT INTO country (code, name, active) VALUES
    ('IL', 'Israel', true),
    ('US', 'United States', true),
    ('XX', 'Neutral Zone', false);
