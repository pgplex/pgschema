-- base_a gains a column while child leaves base_b: the new column reaches
-- child through base_a, label_b becomes local and is dropped, and
-- shared_check stays inherited from base_a.
CREATE TABLE base_a (
    id integer NOT NULL,
    label_a text,
    added text,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE base_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE child (
    extra integer
) INHERITS (base_a);
