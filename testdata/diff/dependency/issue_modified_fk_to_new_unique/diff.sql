ALTER TABLE parent_variants
ADD CONSTRAINT parent_variants_parent_entity_id_id_key UNIQUE (parent_entity_id, id);

ALTER TABLE child_links DROP CONSTRAINT child_links_parent_variant_fkey;

ALTER TABLE child_links
ADD CONSTRAINT child_links_parent_variant_fkey FOREIGN KEY (parent_entity_id, parent_variant_id) REFERENCES parent_variants (parent_entity_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE child_links VALIDATE CONSTRAINT child_links_parent_variant_fkey;
