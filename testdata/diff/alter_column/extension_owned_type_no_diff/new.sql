-- Same ltree-typed column as old.sql, but ltree is installed into a
-- different schema (ext_schema_b instead of ext_schema_a). This mirrors the
-- real-world scenario where a plan-time temp schema can't relocate an
-- extension into the same schema it lives in on the live DB. The column
-- itself is unchanged, so no diff should be generated.
--
-- Extensions are database-wide (not scoped to the "public" schema the test
-- harness resets between old.sql/new.sql), so ltree is dropped and
-- reinstalled here to guarantee a clean, schema-mismatched state each time.
DROP EXTENSION IF EXISTS ltree CASCADE;
DROP SCHEMA IF EXISTS ext_schema_a CASCADE;
DROP SCHEMA IF EXISTS ext_schema_b CASCADE;
CREATE SCHEMA ext_schema_b;
CREATE EXTENSION ltree SCHEMA ext_schema_b;

CREATE TABLE documents (
    id bigint PRIMARY KEY,
    path ext_schema_b.ltree
);
