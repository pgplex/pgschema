CREATE TYPE datetimeoffset AS (local_time timestamp without time zone, offset_minutes smallint);

CREATE TABLE account_groups (
    id uuid NOT NULL,
    company_id uuid NOT NULL,
    name varchar NOT NULL,
    created_at datetimeoffset NOT NULL,
    updated_at datetimeoffset NOT NULL
);

CREATE OR REPLACE FUNCTION get_account_group_by_id(
    p_group_id uuid
)
RETURNS TABLE(id uuid, company_id uuid, name varchar, created_at datetimeoffset, updated_at datetimeoffset)
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    SELECT
        ag.id,
        ag.company_id,
        ag.name,
        ag.created_at,
        ag.updated_at
    FROM account_groups ag
    WHERE ag.id = p_group_id;
END;
$$;
