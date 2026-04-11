CREATE OR REPLACE FUNCTION role_has_cap(
    p_role role_type,
    p_cap text
)
RETURNS boolean
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
