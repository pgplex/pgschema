DROP FUNCTION IF EXISTS fn_create_user(text);

ALTER TABLE tb_users ADD COLUMN role text DEFAULT 'member' NOT NULL;

DROP VIEW IF EXISTS vw_users RESTRICT;

CREATE OR REPLACE VIEW vw_users AS
 SELECT id,
    email,
    role,
    created_at
   FROM tb_users
  WHERE is_deleted = false;

CREATE OR REPLACE FUNCTION fn_create_user(
    email text,
    role text DEFAULT 'member'
)
RETURNS vw_users
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
AS $$
DECLARE
    v_result vw_users;
    v_new_id UUID;
BEGIN
    INSERT INTO tb_users (email, role) VALUES (email, role) RETURNING id INTO v_new_id;
    SELECT id, email, role, created_at INTO v_result FROM vw_users WHERE id = v_new_id;
    RETURN v_result;
END;
$$;
