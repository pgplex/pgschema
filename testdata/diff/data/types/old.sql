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

INSERT INTO setting (id, name, ratio, tags, payload, updated_at, blob, note) OVERRIDING SYSTEM VALUE VALUES
    (1, 'alpha', 1.5, '{a,b}', '{"k": 1}', '2024-01-01T00:00:00Z', '\x00ff', 'it''s'),
    (2, 'beta', NULL, NULL, NULL, NULL, NULL, '');
