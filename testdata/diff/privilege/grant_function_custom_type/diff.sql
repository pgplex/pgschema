REVOKE EXECUTE ON FUNCTION create_entity(p_name text, p_kind entity_kind) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION create_entity(p_name text, p_kind entity_kind) TO app_user;
