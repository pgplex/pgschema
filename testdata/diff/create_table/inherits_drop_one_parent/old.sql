CREATE TABLE base_a (
    id integer NOT NULL,
    label_a text,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE base_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE child (
    extra integer
) INHERITS (base_a, base_b);
