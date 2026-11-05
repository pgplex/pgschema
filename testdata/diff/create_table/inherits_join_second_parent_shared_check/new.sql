-- child already has shared_check through base_a, so joining base_b needs
-- nothing but the INHERIT.
CREATE TABLE base_a (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE base_b (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE child (
    extra text
) INHERITS (base_a, base_b);
