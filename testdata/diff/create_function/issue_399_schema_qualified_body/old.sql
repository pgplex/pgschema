CREATE TYPE role_type AS ENUM ('OWNER', 'MEMBER');

CREATE TABLE role_caps (
    role role_type NOT NULL,
    capability text NOT NULL,
    PRIMARY KEY (role, capability)
);
