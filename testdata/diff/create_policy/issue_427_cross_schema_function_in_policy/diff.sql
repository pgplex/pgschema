CREATE POLICY admin_all_items ON items TO authenticated USING (auth.role() = 'admin');
CREATE POLICY select_own_items ON items FOR SELECT TO authenticated USING (owner_id = ( SELECT auth.uid() AS uid));
