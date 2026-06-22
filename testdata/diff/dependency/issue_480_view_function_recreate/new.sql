CREATE TABLE tb_users (
    id uuid DEFAULT gen_random_uuid(),
    is_deleted boolean DEFAULT false NOT NULL,
    created_at timestamptz DEFAULT now(),
    email text DEFAULT 'missing@missing.com' NOT NULL,
    role text DEFAULT 'member' NOT NULL,
    CONSTRAINT tb_users_pkey PRIMARY KEY (id)
);

CREATE VIEW vw_users AS
SELECT id, email, role, created_at
FROM tb_users
WHERE is_deleted = FALSE;

CREATE FUNCTION fn_create_user(email TEXT, role TEXT DEFAULT 'member')
RETURNS vw_users AS $$
DECLARE
    v_result vw_users;
    v_new_id UUID;
BEGIN
    INSERT INTO tb_users (email, role) VALUES (email, role) RETURNING id INTO v_new_id;
    SELECT id, email, role, created_at INTO v_result FROM vw_users WHERE id = v_new_id;
    RETURN v_result;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
