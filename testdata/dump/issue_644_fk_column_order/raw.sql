-- Two tables with a same-named foreign key (fk_p) on the same columns but in
-- different order. Foreign key names only have to be unique per table, so a
-- constraint-column position lookup keyed on (schema, constraint name, column)
-- without the table can return the other table's position and dump c2 with
-- c1's column order (GitHub issue #644).
CREATE TABLE p (
    a int,
    b int,
    PRIMARY KEY (a, b),
    UNIQUE (b, a)
);

CREATE TABLE c1 (
    a int,
    b int,
    CONSTRAINT fk_p FOREIGN KEY (a, b) REFERENCES p (a, b)
);

CREATE TABLE c2 (
    a int,
    b int,
    CONSTRAINT fk_p FOREIGN KEY (b, a) REFERENCES p (b, a)
);
