CREATE TYPE severity AS ENUM (
    'low',
    'medium',
    'high'
);

CREATE TYPE band AS RANGE (
    subtype = severity
);
