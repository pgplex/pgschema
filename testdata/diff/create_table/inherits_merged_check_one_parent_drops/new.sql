-- base_a drops shared_check; child still inherits it from base_b, so its
-- copy stays.
CREATE TABLE base_a (
    id integer NOT NULL
);

CREATE TABLE base_b (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE child (
    extra text,
    CONSTRAINT shared_check CHECK (id > 0)
) INHERITS (base_a, base_b);
