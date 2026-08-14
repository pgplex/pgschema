CREATE TABLE IF NOT EXISTS parent_orgs (
    id bigint,
    external_key uuid NOT NULL,
    parent_id bigint,
    migrated_from_external_key uuid,
    CONSTRAINT parent_orgs_pkey PRIMARY KEY (id),
    CONSTRAINT parent_orgs_external_key_key UNIQUE (external_key),
    CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES parent_orgs (id)
);

ALTER TABLE parent_orgs
ADD CONSTRAINT fk_self_migrated_from FOREIGN KEY (migrated_from_external_key) REFERENCES parent_orgs (external_key);
