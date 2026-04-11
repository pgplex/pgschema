CREATE TYPE public.role_type AS ENUM ('OWNER', 'MEMBER');

CREATE TABLE public.role_caps (
    role public.role_type NOT NULL,
    capability text NOT NULL,
    PRIMARY KEY (role, capability)
);

CREATE OR REPLACE FUNCTION public.role_has_cap(
    p_role public.role_type,
    p_cap text
) RETURNS boolean
LANGUAGE sql
STABLE
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM public.role_caps rc
        WHERE rc.role = p_role
          AND rc.capability = p_cap
    );
$$;
