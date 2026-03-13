CREATE TABLE IF NOT EXISTS users (
    id integer,
    email varchar(255) NOT NULL,
    name varchar(100),
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email varchar_pattern_ops);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique ON users (email) NULLS NOT DISTINCT;

CREATE INDEX IF NOT EXISTS idx_users_id ON users (id);

CREATE INDEX IF NOT EXISTS idx_users_name ON users (name);

CREATE INDEX IF NOT EXISTS "public.idx_users" ON users (email, name);
