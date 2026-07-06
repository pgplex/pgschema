CREATE TABLE IF NOT EXISTS vt (
    slug text NOT NULL,
    identifier text GENERATED ALWAYS AS (('urn:sdp:catalog:'::text || slug)) VIRTUAL
);
