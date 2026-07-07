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

CREATE TABLE parent2 (
    id uuid PRIMARY KEY,
    tenant text NOT NULL
);

CREATE UNIQUE INDEX parent2_id_tenant_key ON parent2 (id, tenant);

CREATE TABLE child2 (
    id uuid PRIMARY KEY,
    parent2_id uuid,
    tenant text NOT NULL,
    FOREIGN KEY (parent2_id, tenant) REFERENCES parent2 (id, tenant)
);
