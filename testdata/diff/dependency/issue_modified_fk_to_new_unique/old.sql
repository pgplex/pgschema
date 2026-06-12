CREATE TABLE parent_entities (
    id integer PRIMARY KEY
);

CREATE TABLE parent_variants (
    id integer PRIMARY KEY,
    parent_entity_id integer NOT NULL,
    CONSTRAINT parent_variants_parent_entity_id_fkey FOREIGN KEY (parent_entity_id)
        REFERENCES parent_entities (id)
        ON DELETE CASCADE
);

CREATE TABLE child_links (
    parent_variant_id integer NOT NULL,
    parent_entity_id integer NOT NULL,
    CONSTRAINT child_links_parent_variant_fkey FOREIGN KEY (parent_variant_id)
        REFERENCES parent_variants (id)
        ON DELETE CASCADE
);
