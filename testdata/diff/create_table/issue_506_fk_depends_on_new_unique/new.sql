CREATE TABLE parent (
    id uuid PRIMARY KEY,
    tenant text NOT NULL,
    UNIQUE (id, tenant)
);

CREATE TABLE child (
    id uuid PRIMARY KEY,
    parent_id uuid,
    tenant text NOT NULL,
    FOREIGN KEY (parent_id, tenant) REFERENCES parent (id, tenant)
);
