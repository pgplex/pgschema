CREATE TABLE base_a (
    shared integer,
    label_a text
);

CREATE TABLE base_b (
    shared integer,
    label_b text
);

-- shared is declared in the child too, merged with both parents' columns
CREATE TABLE child (
    shared integer,
    extra text
) INHERITS (base_a, base_b);
