CREATE TABLE IF NOT EXISTS child (
    id uuid,
    parent_id uuid,
    tenant text NOT NULL,
    CONSTRAINT child_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS child2 (
    id uuid,
    parent2_id uuid,
    tenant text NOT NULL,
    CONSTRAINT child2_pkey PRIMARY KEY (id)
);

ALTER TABLE parent
ADD CONSTRAINT parent_id_tenant_key UNIQUE (id, tenant);

CREATE UNIQUE INDEX IF NOT EXISTS parent2_id_tenant_key ON parent2 (id, tenant);

ALTER TABLE child
ADD CONSTRAINT child_parent_id_tenant_fkey FOREIGN KEY (parent_id, tenant) REFERENCES parent (id, tenant);

ALTER TABLE child2
ADD CONSTRAINT child2_parent2_id_tenant_fkey FOREIGN KEY (parent2_id, tenant) REFERENCES parent2 (id, tenant);
