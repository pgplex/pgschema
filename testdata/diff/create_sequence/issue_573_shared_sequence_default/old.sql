-- Existing table that gains a column backed by a custom-named owned sequence
CREATE TABLE legacy (
    id bigint NOT NULL,
    CONSTRAINT legacy_pkey PRIMARY KEY (id)
);

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
