CREATE SEQUENCE IF NOT EXISTS hibernate_sequence;
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
ALTER SEQUENCE widget_custom_seq OWNED BY widget.id;
