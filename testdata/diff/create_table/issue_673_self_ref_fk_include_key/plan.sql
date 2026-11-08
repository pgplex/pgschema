ALTER TABLE node ADD COLUMN id integer;

ALTER TABLE node ADD COLUMN parent_id integer;

ALTER TABLE node
ADD CONSTRAINT node_pkey PRIMARY KEY (id) INCLUDE (payload);

ALTER TABLE node
ADD CONSTRAINT node_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES node (id) NOT VALID;

ALTER TABLE node VALIDATE CONSTRAINT node_parent_id_fkey;
