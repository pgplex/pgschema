CREATE TABLE base_a (
    x integer DEFAULT 1,
    y integer
);

CREATE TABLE base_b (
    x integer DEFAULT 2,
    y integer DEFAULT 5
);

-- x: the parents disagree on the default, so the child must declare it.
-- y: only base_b has a default, which the child inherits and then drops.
CREATE TABLE child (
    x integer DEFAULT 3,
    extra text
) INHERITS (base_a, base_b);

ALTER TABLE ONLY child ALTER COLUMN y DROP DEFAULT;
