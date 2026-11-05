CREATE TABLE base_a (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE base_b (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE child (
    extra text,
    CONSTRAINT shared_check CHECK (id > 0)
) INHERITS (base_a, base_b);
