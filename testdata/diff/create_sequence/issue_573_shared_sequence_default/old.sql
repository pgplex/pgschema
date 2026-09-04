-- Existing table that gains a column backed by a custom-named owned sequence
CREATE TABLE legacy (
    id bigint NOT NULL,
    CONSTRAINT legacy_pkey PRIMARY KEY (id)
);

-- Explicit, unowned sequence pre-created for a table that doesn't exist yet:
-- the desired state adds the table with a SERIAL column of the same name, so
-- the plan must adopt the sequence instead of letting SERIAL recreate it.
CREATE SEQUENCE adopt_new_id_seq AS integer;

-- Explicit, unowned sequence that the desired state turns into a SERIAL column:
-- only the ownership differs, so the plan must attach it.
CREATE SEQUENCE adopt_id_seq AS integer;

CREATE TABLE adopt (
    id integer DEFAULT nextval('adopt_id_seq'::regclass) NOT NULL,
    note text,
    CONSTRAINT adopt_pkey PRIMARY KEY (id)
);

-- SERIAL column that the desired state turns into an explicit, unowned
-- sequence with the same name: the plan must release the ownership.
CREATE TABLE release (
    id serial NOT NULL,
    note text,
    CONSTRAINT release_pkey PRIMARY KEY (id)
);

-- Custom-named owned sequence that becomes unowned in the desired state.
CREATE SEQUENCE tracker_custom_seq;

CREATE TABLE tracker (
    n integer DEFAULT nextval('tracker_custom_seq'::regclass) NOT NULL,
    note text,
    CONSTRAINT tracker_pkey PRIMARY KEY (n)
);

ALTER SEQUENCE tracker_custom_seq OWNED BY tracker.n;

-- Owned sequence whose owning table is dropped while the sequence is kept:
-- it must be released before the DROP TABLE cascades to it.
CREATE SEQUENCE keep_seq;

CREATE TABLE doomed (
    id integer DEFAULT nextval('keep_seq'::regclass) NOT NULL,
    CONSTRAINT doomed_pkey PRIMARY KEY (id)
);

ALTER SEQUENCE keep_seq OWNED BY doomed.id;

-- Owned sequence re-pointed to another column while its old owning column is
-- dropped: release first, re-point after the column changes.
CREATE SEQUENCE move_seq;

CREATE TABLE mover (
    a integer DEFAULT nextval('move_seq'::regclass) NOT NULL,
    b integer NOT NULL,
    CONSTRAINT mover_pkey PRIMARY KEY (b)
);

ALTER SEQUENCE move_seq OWNED BY mover.a;
