DROP FUNCTION IF EXISTS fn_create_user(text);

CREATE OR REPLACE VIEW vw_active AS
 SELECT id
   FROM tb_users
  WHERE is_deleted = false;

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

CREATE OR REPLACE FUNCTION fn_pair_step(
    state bigint,
    u vw_users,
    a vw_active
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + 1
$$;

CREATE OR REPLACE FUNCTION fn_users_step(
    state bigint,
    u vw_users
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + 1
$$;

CREATE AGGREGATE agg_pair(vw_users, vw_active) (
    SFUNC = fn_pair_step,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE AGGREGATE agg_users_count(vw_users) (
    SFUNC = fn_users_step,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE OR REPLACE FUNCTION active_marker(
    a vw_active
)
RETURNS bigint
LANGUAGE sql
VOLATILE
AS $$ SELECT agg_users_count(NULL::vw_users)
$$;

CREATE OR REPLACE VIEW vw_users_count AS
 SELECT agg_users_count(vw_users.*) AS n
   FROM vw_users;

CREATE OR REPLACE VIEW vw_role_counts AS
 SELECT u.role,
    agg_users_count(vw_users.*) AS n
   FROM tb_users u
     JOIN vw_users ON vw_users.id = u.id
  GROUP BY u.role;
