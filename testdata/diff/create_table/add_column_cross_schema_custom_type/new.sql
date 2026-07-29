-- New state: Add columns using extension types, custom domain, and enum
-- Types are created via setup.sql
--
-- This tests GitHub #197 and #144/#145:
--
-- PUBLIC schema type (citext):
--   Written unqualified because both table and type are in public schema.
--   This is natural usage - pgschema must handle this by including public
--   in search_path when applying to temp schema.
--
-- CUSTOM schema types (hstore, custom_text, custom_enum):
--   Written with schema qualifier because they're in utils schema, and the
--   emitted DDL keeps it qualified: hstore is extension-owned, but the
--   extension-owned-type schema-qualifier fix qualifies with wherever the
--   extension is genuinely installed on the live database (utils here, per
--   setup.sql), not whatever schema plan-time introspection happened to
--   resolve it to - so this is no different from custom_text/custom_enum,
--   ordinary (non-extension) types whose schema is likewise preserved.

CREATE TABLE public.users (
    id bigint PRIMARY KEY,
    username text NOT NULL,
    created_at timestamp DEFAULT CURRENT_TIMESTAMP,
    -- Extension type from public schema: unqualified (natural usage)
    -- Reproduces #197 - pgschema must include public in search_path
    fqdn citext NOT NULL,
    -- Extension type from utils schema: schema-qualified here and in the
    -- emitted DDL (see diff.sql) - utils is where the extension is genuinely
    -- installed on the live database, per setup.sql.
    metadata utils.hstore,
    -- Custom domain from utils schema: must be schema-qualified
    description utils.custom_text,
    -- Enum type from utils schema: must be schema-qualified
    status utils.custom_enum DEFAULT 'active'
);
