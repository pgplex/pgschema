CREATE TABLE parent_orgs (
    id bigint PRIMARY KEY,
    external_key uuid NOT NULL UNIQUE,
    migrated_from_external_key uuid,
    CONSTRAINT fk_self_migrated_from
        FOREIGN KEY (migrated_from_external_key) REFERENCES parent_orgs (external_key)
);
