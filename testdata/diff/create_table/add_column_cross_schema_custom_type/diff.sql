ALTER TABLE users ADD COLUMN fqdn citext NOT NULL;
ALTER TABLE users ADD COLUMN metadata utils.hstore;
ALTER TABLE users ADD COLUMN description utils.custom_text;
ALTER TABLE users ADD COLUMN status utils.custom_enum DEFAULT 'active'::utils.custom_enum;
