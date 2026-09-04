-- Issue #573/#574/#576: sequences referenced by column defaults but not owned by
-- the column must be emitted explicitly. Only sequences genuinely created by
-- SERIAL (owned via pg_depend and named <table>_<column>_seq) may be collapsed
-- into the SERIAL shorthand.

-- One shared sequence feeding several tables (Hibernate-style global id)
CREATE SEQUENCE nsl_global_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE hibernate_sequence
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE TABLE instance (
    id bigint DEFAULT nextval('nsl_global_seq'::regclass) NOT NULL,
    name text,
    CONSTRAINT instance_pkey PRIMARY KEY (id)
);

CREATE TABLE reference (
    id bigint DEFAULT nextval('nsl_global_seq'::regclass) NOT NULL,
    title text,
    CONSTRAINT reference_pkey PRIMARY KEY (id)
);

CREATE TABLE shard_config (
    id bigint DEFAULT nextval('hibernate_sequence'::regclass) NOT NULL,
    name character varying(255) NOT NULL,
    CONSTRAINT shard_config_pkey PRIMARY KEY (id)
);

-- Sequence owned by the column but with a custom name: CREATE TABLE ... SERIAL
-- would create widget_id_seq instead, so it must stay explicit as well.
CREATE SEQUENCE widget_custom_seq;

CREATE TABLE widget (
    id integer DEFAULT nextval('widget_custom_seq'::regclass) NOT NULL,
    label text,
    CONSTRAINT widget_pkey PRIMARY KEY (id)
);

ALTER SEQUENCE widget_custom_seq OWNED BY widget.id;

-- Genuine SERIAL column for contrast: keeps the shorthand
CREATE TABLE audit_log (
    id bigserial NOT NULL,
    message text,
    CONSTRAINT audit_log_pkey PRIMARY KEY (id)
);

-- Existing table gains a column whose default is a custom-named sequence that
-- is OWNED BY the new column: the sequence must be created before the column
-- default references it, and the OWNED BY applied after ALTER TABLE ADD COLUMN.
CREATE SEQUENCE legacy_code_custom_seq;

CREATE TABLE legacy (
    id bigint NOT NULL,
    code integer DEFAULT nextval('legacy_code_custom_seq'::regclass) NOT NULL,
    CONSTRAINT legacy_pkey PRIMARY KEY (id)
);

ALTER SEQUENCE legacy_code_custom_seq OWNED BY legacy.code;

-- Ownership-only transitions against old.sql (see comments there)
CREATE TABLE adopt (
    id serial NOT NULL,
    note text,
    CONSTRAINT adopt_pkey PRIMARY KEY (id)
);

-- Brand new table adopting a sequence that already exists in the old state
-- (see comments there): the SERIAL column must not try to recreate it.
CREATE TABLE adopt_new (
    id serial NOT NULL,
    note text,
    CONSTRAINT adopt_new_pkey PRIMARY KEY (id)
);

CREATE SEQUENCE release_id_seq AS integer;

CREATE TABLE release (
    id integer DEFAULT nextval('release_id_seq'::regclass) NOT NULL,
    note text,
    CONSTRAINT release_pkey PRIMARY KEY (id)
);

CREATE SEQUENCE tracker_custom_seq;

CREATE TABLE tracker (
    n integer DEFAULT nextval('tracker_custom_seq'::regclass) NOT NULL,
    note text,
    CONSTRAINT tracker_pkey PRIMARY KEY (n)
);

-- Owner drops against old.sql (see comments there)
CREATE SEQUENCE keep_seq;

CREATE SEQUENCE move_seq;

CREATE TABLE mover (
    b integer DEFAULT nextval('move_seq'::regclass) NOT NULL,
    CONSTRAINT mover_pkey PRIMARY KEY (b)
);

ALTER SEQUENCE move_seq OWNED BY mover.b;
