ALTER SEQUENCE keep_seq OWNED BY NONE;

ALTER SEQUENCE move_seq OWNED BY NONE;

DROP TABLE IF EXISTS doomed CASCADE;

CREATE SEQUENCE IF NOT EXISTS hibernate_sequence;

CREATE SEQUENCE IF NOT EXISTS legacy_code_custom_seq;

CREATE SEQUENCE IF NOT EXISTS nsl_global_seq;

CREATE SEQUENCE IF NOT EXISTS widget_custom_seq;

CREATE TABLE IF NOT EXISTS audit_log (
    id BIGSERIAL,
    message text,
    CONSTRAINT audit_log_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS instance (
    id bigint DEFAULT nextval('nsl_global_seq'::regclass),
    name text,
    CONSTRAINT instance_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS reference (
    id bigint DEFAULT nextval('nsl_global_seq'::regclass),
    title text,
    CONSTRAINT reference_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS shard_config (
    id bigint DEFAULT nextval('hibernate_sequence'::regclass),
    name varchar(255) NOT NULL,
    CONSTRAINT shard_config_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS widget (
    id integer DEFAULT nextval('widget_custom_seq'::regclass),
    label text,
    CONSTRAINT widget_pkey PRIMARY KEY (id)
);

ALTER TABLE legacy ADD COLUMN code integer DEFAULT nextval('legacy_code_custom_seq'::regclass) NOT NULL;

ALTER TABLE mover DROP COLUMN a;

ALTER TABLE mover ALTER COLUMN b SET DEFAULT nextval('move_seq'::regclass);

ALTER SEQUENCE legacy_code_custom_seq OWNED BY legacy.code;

ALTER SEQUENCE widget_custom_seq OWNED BY widget.id;

ALTER SEQUENCE adopt_id_seq OWNED BY adopt.id;

ALTER SEQUENCE move_seq OWNED BY mover.b;

ALTER SEQUENCE release_id_seq OWNED BY NONE;

ALTER SEQUENCE tracker_custom_seq OWNED BY NONE;
