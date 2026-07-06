CREATE TABLE IF NOT EXISTS products (
    id SERIAL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT products_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS users (
    id SERIAL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT users_pkey PRIMARY KEY (id),
    CONSTRAINT _template_timestamps_check CHECK (created_at <= updated_at)
);

COMMENT ON COLUMN users.created_at IS 'Record creation time';

CREATE INDEX IF NOT EXISTS users_created_at_idx ON users (created_at);