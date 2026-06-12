CREATE TABLE z_parent_entities (
    id integer PRIMARY KEY
);

CREATE TABLE z_parent_variants (
    id integer PRIMARY KEY,
    parent_entity_id integer NOT NULL,
    CONSTRAINT z_parent_variants_parent_entity_id_fkey FOREIGN KEY (parent_entity_id)
        REFERENCES z_parent_entities (id)
        ON DELETE CASCADE,
    CONSTRAINT z_parent_variants_parent_entity_id_id_key UNIQUE (parent_entity_id, id)
);

CREATE TABLE a_child_links (
    parent_variant_id integer NOT NULL,
    parent_entity_id integer NOT NULL,
    CONSTRAINT a_child_links_parent_variant_fkey FOREIGN KEY (parent_entity_id, parent_variant_id)
        REFERENCES z_parent_variants (parent_entity_id, id)
        ON DELETE CASCADE
);
