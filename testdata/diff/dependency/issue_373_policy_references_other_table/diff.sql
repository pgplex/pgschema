CREATE TABLE IF NOT EXISTS manager (
    id SERIAL,
    user_id uuid NOT NULL
);

ALTER TABLE manager ENABLE ROW LEVEL SECURITY;

CREATE TABLE IF NOT EXISTS project_manager (
    id SERIAL,
    project_id integer NOT NULL,
    manager_id integer NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL
);

CREATE POLICY employee_manager_select ON manager FOR SELECT TO PUBLIC USING (id IN ( SELECT pam.manager_id FROM project_manager pam WHERE ((pam.project_id IN ( SELECT unnest(ARRAY[1, 2, 3]) AS unnest)) AND (pam.is_deleted = false))));
