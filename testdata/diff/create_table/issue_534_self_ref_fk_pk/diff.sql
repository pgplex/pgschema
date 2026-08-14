CREATE TABLE IF NOT EXISTS categories (
    id bigint,
    name text NOT NULL,
    parent_id bigint,
    CONSTRAINT categories_pkey PRIMARY KEY (id),
    CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES categories (id)
);
