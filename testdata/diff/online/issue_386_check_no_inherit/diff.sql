ALTER TABLE parent_base
ADD CONSTRAINT no_direct_insert CHECK (false) NO INHERIT;
