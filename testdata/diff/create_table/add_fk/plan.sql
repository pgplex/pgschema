ALTER TABLE books
ADD CONSTRAINT books_author_id_fkey FOREIGN KEY (author_id) REFERENCES authors (id) ON DELETE CASCADE NOT VALID;

ALTER TABLE books VALIDATE CONSTRAINT books_author_id_fkey;

ALTER TABLE employees
ADD CONSTRAINT employees_department_id_fkey FOREIGN KEY (department_id) REFERENCES departments (id) NOT VALID;

ALTER TABLE employees VALIDATE CONSTRAINT employees_department_id_fkey;

ALTER TABLE nodes
ADD CONSTRAINT nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES nodes (id) NOT VALID;

ALTER TABLE nodes VALIDATE CONSTRAINT nodes_parent_id_fkey;

ALTER TABLE orders
ADD CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id) NOT VALID;

ALTER TABLE orders VALIDATE CONSTRAINT orders_customer_id_fkey;

ALTER TABLE orders
ADD CONSTRAINT orders_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES managers (id) ON DELETE SET NULL NOT VALID;

ALTER TABLE orders VALIDATE CONSTRAINT orders_manager_id_fkey;

ALTER TABLE orders
ADD CONSTRAINT orders_product_id_fkey FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE NOT VALID;

ALTER TABLE orders VALIDATE CONSTRAINT orders_product_id_fkey;

ALTER TABLE price_adjustments
ADD CONSTRAINT price_adjustments_product_fkey FOREIGN KEY (product_id, PERIOD adjustment_period) REFERENCES price_history (product_id, PERIOD valid_period) NOT VALID;

ALTER TABLE price_adjustments VALIDATE CONSTRAINT price_adjustments_product_fkey;

ALTER TABLE products
ADD CONSTRAINT products_category_code_fkey FOREIGN KEY (category_code) REFERENCES categories (code) ON UPDATE CASCADE NOT VALID;

ALTER TABLE products VALIDATE CONSTRAINT products_category_code_fkey;

ALTER TABLE projects
ADD CONSTRAINT projects_tenant_id_org_id_fkey FOREIGN KEY (tenant_id, org_id) REFERENCES organizations (tenant_id, org_id) NOT VALID;

ALTER TABLE projects VALIDATE CONSTRAINT projects_tenant_id_org_id_fkey;

ALTER TABLE teams
ADD CONSTRAINT teams_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES managers (id) ON DELETE SET NULL NOT VALID;

ALTER TABLE teams VALIDATE CONSTRAINT teams_manager_id_fkey;

ALTER TABLE user_profiles
ADD CONSTRAINT user_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) DEFERRABLE INITIALLY DEFERRED NOT VALID;

ALTER TABLE user_profiles VALIDATE CONSTRAINT user_profiles_user_id_fkey;
