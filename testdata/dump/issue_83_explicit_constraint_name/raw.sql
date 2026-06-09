--
-- Test case for GitHub issue #83: Named constraint preservation
--
-- This test verifies that ALL constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK)
-- are output as table-level constraints with explicit names in dump output.
--
-- Expected behavior in DUMP output:
-- - ALL constraints output as table-level with explicit CONSTRAINT names
-- - Explicitly named constraints preserve their custom names (e.g., fk_rails_8bfc3d9c01)
-- - Auto-generated names follow PostgreSQL conventions (e.g., table_column_fkey)
-- - No inline constraint syntax in dump output
--
-- Note: Inline syntax (e.g., "id integer PRIMARY KEY") is only used in
-- migration/diff output when adding single-column constraints via ALTER TABLE ADD COLUMN.
--

--
-- Case 1: Explicitly named foreign key (from the original issue)
-- This is the problematic case - the name should be preserved
--
CREATE TABLE some_table (
    id serial PRIMARY KEY,
    name varchar(255) NOT NULL
);

CREATE TABLE some_other_table (
    id serial PRIMARY KEY,
    some_table_id integer NOT NULL
);

-- Named constraint - should preserve the explicit name "fk_rails_8bfc3d9c01"
ALTER TABLE ONLY some_other_table
    ADD CONSTRAINT fk_rails_8bfc3d9c01
    FOREIGN KEY (some_table_id) REFERENCES some_table(id);


--
-- Case 2: Multiple named foreign keys with different actions
-- Tests that various FK configurations preserve their names
--
CREATE TABLE departments (
    id serial PRIMARY KEY,
    name varchar(255) NOT NULL
);

CREATE TABLE employees (
    id serial PRIMARY KEY,
    name varchar(255) NOT NULL,
    department_id integer,
    manager_id integer
);

-- Named FK with ON DELETE CASCADE
ALTER TABLE ONLY employees
    ADD CONSTRAINT fk_employee_department
    FOREIGN KEY (department_id) REFERENCES departments(id) ON DELETE CASCADE;

-- Named FK with ON DELETE SET NULL
ALTER TABLE ONLY employees
    ADD CONSTRAINT fk_employee_manager
    FOREIGN KEY (manager_id) REFERENCES employees(id) ON DELETE SET NULL;


--
-- Case 3: Composite foreign key with explicit name
-- Tests multi-column foreign keys preserve names
--
CREATE TABLE projects (
    project_id integer NOT NULL,
    phase_id integer NOT NULL,
    name varchar(255) NOT NULL,
    PRIMARY KEY (project_id, phase_id)
);

CREATE TABLE tasks (
    id serial PRIMARY KEY,
    project_id integer NOT NULL,
    phase_id integer NOT NULL,
    description text
);

-- Composite FK with explicit name
ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_task_project_phase
    FOREIGN KEY (project_id, phase_id) REFERENCES projects(project_id, phase_id);


--
-- Case 4: Named FK with DEFERRABLE option
-- Tests that constraint options are preserved along with names
--
CREATE TABLE authors (
    id serial PRIMARY KEY,
    name varchar(255) NOT NULL
);

CREATE TABLE books (
    id serial PRIMARY KEY,
    title varchar(255) NOT NULL,
    author_id integer NOT NULL
);

-- Named FK with DEFERRABLE INITIALLY DEFERRED
ALTER TABLE ONLY books
    ADD CONSTRAINT fk_book_author_deferred
    FOREIGN KEY (author_id) REFERENCES authors(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Case 5: Named UNIQUE constraint with DEFERRABLE option (issue #404)
-- Tests that DEFERRABLE INITIALLY DEFERRED is preserved on non-FK constraints,
-- not just foreign keys
--
ALTER TABLE ONLY authors
    ADD CONSTRAINT authors_name_key UNIQUE (name) DEFERRABLE INITIALLY DEFERRED;
