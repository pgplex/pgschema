CREATE TABLE items (
    id BIGSERIAL PRIMARY KEY,
    owner_id uuid NOT NULL,
    name text NOT NULL
);

ALTER TABLE items ENABLE ROW LEVEL SECURITY;

-- Policy using auth.uid()
CREATE POLICY select_own_items
ON items
FOR SELECT
TO authenticated
USING (owner_id = (SELECT auth.uid()));

-- Policy using auth.role()
CREATE POLICY admin_all_items
ON items
FOR ALL
TO authenticated
USING (auth.role() = 'admin');
