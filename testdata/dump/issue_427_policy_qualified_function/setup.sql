--
-- Setup: Supabase-style cross-schema RLS policy (GitHub issue #427).
--
-- A helper schema (auth) exposes functions that are referenced from RLS
-- policies in a managed schema (app). The helper schema is placed on the
-- connection search_path, which is what makes pg_get_expr (via the
-- pg_policies view) drop the schema qualifier and emit a bare uid().
--

CREATE SCHEMA auth;

CREATE FUNCTION auth.uid() RETURNS uuid
    LANGUAGE sql STABLE
    AS $$ SELECT '00000000-0000-0000-0000-000000000000'::uuid $$;

CREATE FUNCTION auth.role() RETURNS text
    LANGUAGE sql STABLE
    AS $$ SELECT 'authenticated'::text $$;

CREATE SCHEMA app;

CREATE TABLE app.items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id uuid NOT NULL,
    name text NOT NULL
);

ALTER TABLE app.items ENABLE ROW LEVEL SECURITY;

CREATE POLICY items_select_own ON app.items
    FOR SELECT
    USING (owner_id = (SELECT auth.uid() AS uid));

CREATE POLICY items_modify_role ON app.items
    FOR ALL
    USING (auth.role() = 'authenticated'::text)
    WITH CHECK (auth.role() = 'authenticated'::text);
