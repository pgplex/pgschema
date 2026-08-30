--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.5.1


--
-- Name: process_user_data(uuid, text, text, boolean); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION process_user_data(
    user_id uuid,
    user_name text,
    user_email text,
    is_active boolean
)
RETURNS void
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    -- no-op
END;
$$;

--
-- Name: process_user_data(user_id uuid, user_name text, user_email text, is_active boolean); Type: PRIVILEGE; Schema: privileges; Owner: -
--

GRANT EXECUTE ON FUNCTION process_user_data(user_id uuid, user_name text, user_email text, is_active boolean) TO api_role;

