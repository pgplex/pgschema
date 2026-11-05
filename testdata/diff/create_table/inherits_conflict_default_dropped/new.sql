CREATE TABLE base_a (
    x integer DEFAULT 1
);

CREATE TABLE base_b (
    x integer DEFAULT 2
);

-- The child settled the parents' conflicting defaults and then dropped its
-- own: it has no default, but its declaration must still settle the conflict.
CREATE TABLE child (
    x integer DEFAULT 3,
    extra text
) INHERITS (base_a, base_b);

ALTER TABLE ONLY child ALTER COLUMN x DROP DEFAULT;
