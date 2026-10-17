-- Function whose return type changes (DROP + CREATE)
CREATE FUNCTION default_priority() RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

-- Enum gaining a label: ADD VALUE commits right away, which must come before
-- the held drops, not between them and the restores
CREATE TYPE ticket_state AS ENUM ('open', 'closed');

CREATE TABLE tickets (
    id integer PRIMARY KEY,
    state ticket_state DEFAULT 'open',
    priority integer DEFAULT default_priority(),
    CONSTRAINT tickets_priority_check CHECK (priority <= default_priority() * 10)
);
