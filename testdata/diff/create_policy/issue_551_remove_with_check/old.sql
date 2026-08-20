CREATE TABLE public.agents (
    id integer PRIMARY KEY,
    organization_id uuid NOT NULL
);

ALTER TABLE public.agents ENABLE ROW LEVEL SECURITY;

CREATE POLICY agents_app_tenant ON public.agents
    FOR ALL
    TO PUBLIC
    USING (organization_id = '00000000-0000-0000-0000-000000000001'::uuid)
    WITH CHECK (organization_id = '00000000-0000-0000-0000-000000000001'::uuid);
