-- Reproduces the extension-owned-type schema-qualifier false positive:
-- ltree is installed into a schema ("ext_schema_a" here) other than the
-- table's own schema. On the "old" (live) side that schema happens to be
-- ext_schema_a; on the "new" (desired/plan) side (see new.sql) it's
-- ext_schema_b. Both sides are still the same extension type, so this must
-- not be reported as a diff (see typesEqual/columnsEqual's sameExtensionType
-- bypass).
--
-- ltree is used here (rather than hstore/citext, also used elsewhere in this
-- testdata suite) so moving it between schemas across old.sql/new.sql can't
-- disturb other fixtures that depend on hstore/citext living in a fixed
-- schema for the lifetime of the shared test-suite Postgres instance.
--
-- Extensions are database-wide (not scoped to the "public" schema the test
-- harness resets between old.sql/new.sql), so ltree is dropped and
-- reinstalled here to guarantee a clean, schema-mismatched state each time.
DROP EXTENSION IF EXISTS ltree CASCADE;
DROP SCHEMA IF EXISTS ext_schema_a CASCADE;
DROP SCHEMA IF EXISTS ext_schema_b CASCADE;
CREATE SCHEMA ext_schema_a;
CREATE EXTENSION ltree SCHEMA ext_schema_a;

CREATE TABLE documents (
    id bigint PRIMARY KEY,
    path ext_schema_a.ltree
);
