CREATE TABLE base_a (
    id integer NOT NULL,
    label_a text
);

CREATE TABLE base_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT base_b_id_check CHECK (id > 0)
);

-- child moves from base_a to base_b: label_a becomes local on NO INHERIT and
-- is dropped, label_b and base_b's CHECK are needed before INHERIT.
CREATE TABLE child (
    extra integer
) INHERITS (base_b);
