CREATE TABLE IF NOT EXISTS base_a (
    x integer DEFAULT 1
);

CREATE TABLE IF NOT EXISTS base_b (
    x integer DEFAULT 2
);

CREATE TABLE IF NOT EXISTS child (
    x integer DEFAULT NULL,
    extra text
) INHERITS (base_a, base_b);

ALTER TABLE ONLY child ALTER COLUMN x DROP DEFAULT;
