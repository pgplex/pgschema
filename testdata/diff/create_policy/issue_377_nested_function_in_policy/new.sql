CREATE FUNCTION get_user_assigned_projects() RETURNS integer[]
LANGUAGE sql STABLE AS $$ SELECT ARRAY[1, 2, 3] $$;

CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT
);

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;

-- Policy unchanged, but table has new column
CREATE POLICY project_access ON projects
    FOR SELECT
    TO PUBLIC
    USING (id IN (SELECT unnest(get_user_assigned_projects()) AS unnest));
