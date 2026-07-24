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
--   Written with schema qualifier because they're in utils schema. hstore is
--   extension-owned, though, so the emitted DDL drops the qualifier anyway
--   (see the extension-owned-type schema-qualifier fix): the schema an
--   extension-owned type resolves to during plan-time introspection is an
--   artifact of the temp/plan environment, not a property of any real deploy
--   target, so it's never trustworthy to preserve verbatim - unlike
--   custom_text/custom_enum, which are ordinary (non-extension) types whose
--   schema is a stable, meaningful part of the desired state.

CREATE TABLE public.users (
    id bigint PRIMARY KEY,
    username text NOT NULL,
    created_at timestamp DEFAULT CURRENT_TIMESTAMP,
    -- Extension type from public schema: unqualified (natural usage)
    -- Reproduces #197 - pgschema must include public in search_path
    fqdn citext NOT NULL,
    -- Extension type from utils schema: written schema-qualified here, but
    -- extension-ownership means the emitted DDL strips the qualifier anyway
    -- (see diff.sql) - it's never trustworthy at plan time.
    metadata utils.hstore,
    -- Custom domain from utils schema: must be schema-qualified
    description utils.custom_text,
    -- Enum type from utils schema: must be schema-qualified
    status utils.custom_enum DEFAULT 'active'
);
