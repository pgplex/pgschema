-- population changes type and elevation and cities_pop_check go away; all
-- three changes recurse from the parent to the child.
CREATE TABLE cities (
    name text NOT NULL,
    population bigint,
    CONSTRAINT cities_pkey PRIMARY KEY (name)
);

CREATE TABLE capitals (
    state char(2) NOT NULL
) INHERITS (cities);
