CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL
);

INSERT INTO country (code, name) VALUES ('IL', 'Israel'), ('US', 'United States');
