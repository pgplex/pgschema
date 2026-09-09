ALTER TABLE audit_log
ADD CONSTRAINT audit_log_org_id_actor_member_id_fkey FOREIGN KEY (org_id, actor_member_id) REFERENCES members (org_id, id) ON DELETE SET NULL (actor_member_id);

ALTER TABLE books
ADD CONSTRAINT books_author_id_fkey FOREIGN KEY (author_id) REFERENCES authors (id) ON DELETE CASCADE;

ALTER TABLE employees
ADD CONSTRAINT employees_department_id_fkey FOREIGN KEY (department_id) REFERENCES departments (id);

ALTER TABLE nodes
ADD CONSTRAINT nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES nodes (id);

ALTER TABLE notes DROP CONSTRAINT notes_org_id_author_member_id_fkey;

ALTER TABLE notes
ADD CONSTRAINT notes_org_id_author_member_id_fkey FOREIGN KEY (org_id, author_member_id) REFERENCES members (org_id, id) ON DELETE SET NULL (author_member_id);

ALTER TABLE orders
ADD CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id);

ALTER TABLE orders
ADD CONSTRAINT orders_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES managers (id) ON DELETE SET NULL;

ALTER TABLE orders
ADD CONSTRAINT orders_product_id_fkey FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE;

ALTER TABLE price_adjustments
ADD CONSTRAINT price_adjustments_product_fkey FOREIGN KEY (product_id, PERIOD adjustment_period) REFERENCES price_history (product_id, PERIOD valid_period);

ALTER TABLE products
ADD CONSTRAINT products_category_code_fkey FOREIGN KEY (category_code) REFERENCES categories (code) ON UPDATE CASCADE;

ALTER TABLE projects
ADD CONSTRAINT projects_tenant_id_org_id_fkey FOREIGN KEY (tenant_id, org_id) REFERENCES organizations (tenant_id, org_id);

ALTER TABLE tasks
ADD CONSTRAINT tasks_org_id_owner_member_id_fkey FOREIGN KEY (org_id, owner_member_id) REFERENCES members (org_id, id) ON DELETE SET DEFAULT (owner_member_id);

ALTER TABLE teams
ADD CONSTRAINT teams_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES managers (id) ON DELETE SET NULL;

ALTER TABLE user_profiles
ADD CONSTRAINT user_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) DEFERRABLE INITIALLY DEFERRED;
