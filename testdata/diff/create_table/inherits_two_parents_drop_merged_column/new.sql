-- shared goes from all three tables: the child's merged copy can only be
-- dropped once both parents have dropped theirs.
CREATE TABLE base_a (
    label_a text
);

CREATE TABLE base_b (
    label_b text
);

CREATE TABLE child (
    extra text
) INHERITS (base_a, base_b);
