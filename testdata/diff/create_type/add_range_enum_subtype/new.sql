-- A range over a new enum: the enum must be created first even though the
-- range sorts before it by name.
CREATE TYPE severity AS ENUM ('low', 'medium', 'high');

CREATE TYPE band AS RANGE (subtype = severity);
