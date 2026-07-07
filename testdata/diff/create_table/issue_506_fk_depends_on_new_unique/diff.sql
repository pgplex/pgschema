CREATE TABLE IF NOT EXISTS child (
    id uuid,
    parent_id uuid,
    tenant text NOT NULL,
    CONSTRAINT child_pkey PRIMARY KEY (id)
);

ALTER TABLE parent
ADD CONSTRAINT parent_id_tenant_key UNIQUE (id, tenant);

ALTER TABLE child
ADD CONSTRAINT child_parent_id_tenant_fkey FOREIGN KEY (parent_id, tenant) REFERENCES parent (id, tenant);
