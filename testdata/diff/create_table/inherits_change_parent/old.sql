CREATE TABLE base_a (
    id integer NOT NULL,
    label_a text
);

CREATE TABLE base_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT base_b_id_check CHECK (id > 0)
);

CREATE TABLE child (
    extra integer
) INHERITS (base_a);
