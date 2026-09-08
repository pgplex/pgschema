CREATE TABLE setting (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    ratio numeric(10,2),
    tags text[],
    payload jsonb,
    updated_at timestamptz,
    blob bytea,
    note text,
    upper_name text GENERATED ALWAYS AS (upper(name)) STORED
);

\copy setting (id, name, ratio, tags, payload, updated_at, blob, note) FROM 'data/setting.csv' WITH (FORMAT csv, HEADER)
