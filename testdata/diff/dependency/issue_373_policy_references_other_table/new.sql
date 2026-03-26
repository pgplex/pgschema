CREATE TABLE project_manager (
    id SERIAL,
    project_id int NOT NULL,
    manager_id int NOT NULL,
    is_deleted boolean NOT NULL DEFAULT false
);

CREATE TABLE manager (
    id SERIAL,
    user_id uuid NOT NULL
);

ALTER TABLE manager ENABLE ROW LEVEL SECURITY;

CREATE POLICY employee_manager_select ON manager
    FOR SELECT
    TO PUBLIC
    USING (
        id IN (
            SELECT pam.manager_id
            FROM project_manager pam
            WHERE pam.project_id IN (
                SELECT unnest(ARRAY[1, 2, 3])
            )
            AND pam.is_deleted = false
        )
    );