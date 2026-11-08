CREATE TABLE IF NOT EXISTS accounts (
    id integer,
    name text NOT NULL,
    CONSTRAINT accounts_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS audit_log (
    id integer,
    message text,
    CONSTRAINT audit_log_pkey PRIMARY KEY (id)
);

REVOKE SELECT ON TABLE audit_log FROM app_reader;

GRANT SELECT ON TABLE accounts TO app_reader;
