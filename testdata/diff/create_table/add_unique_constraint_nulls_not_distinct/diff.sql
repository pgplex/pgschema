ALTER TABLE pgschema_repro_nulls
ADD CONSTRAINT pgschema_repro_nulls_uniq UNIQUE NULLS NOT DISTINCT (a, b);
