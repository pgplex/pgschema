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

-- Aggregate whose transition function is typed on the recreated view. Both
-- must be created after the view is recreated in the modify phase (#580).
CREATE FUNCTION fn_users_step(state bigint, u vw_users)
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT state + 1 $$;

CREATE AGGREGATE agg_users_count(vw_users) (
    SFUNC = fn_users_step,
    STYPE = bigint,
    INITCOND = '0'
);

-- Aggregate that depends on a new view AND (through its transition function)
-- on the recreated view: it must still wait for the recreation.
CREATE VIEW vw_active AS
SELECT id FROM tb_users WHERE is_deleted = FALSE;

CREATE FUNCTION fn_pair_step(state bigint, u vw_users, a vw_active)
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT state + 1 $$;

CREATE AGGREGATE agg_pair(vw_users, vw_active) (
    SFUNC = fn_pair_step,
    STYPE = bigint,
    INITCOND = '0'
);

-- New view calling an aggregate held for the recreation: created after it.
CREATE VIEW vw_users_count AS
SELECT agg_users_count(vw_users.*) AS n FROM vw_users;

-- View deferred for the added column (issue #414 path) that also calls an
-- aggregate held for the recreation: it joins the recreated-view batch.
CREATE VIEW vw_role_counts AS
SELECT u.role, agg_users_count(vw_users.*) AS n
FROM tb_users u JOIN vw_users ON vw_users.id = u.id
GROUP BY u.role;
