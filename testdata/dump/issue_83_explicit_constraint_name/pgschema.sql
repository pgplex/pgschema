--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.5
-- Dumped by pgschema version 1.3.0


--
-- Name: authors; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS authors (
    id SERIAL,
    name varchar(255) NOT NULL,
    CONSTRAINT authors_pkey PRIMARY KEY (id),
    CONSTRAINT authors_name_key UNIQUE (name) DEFERRABLE INITIALLY DEFERRED
);

--
-- Name: books; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS books (
    id SERIAL,
    title varchar(255) NOT NULL,
    author_id integer NOT NULL,
    CONSTRAINT books_pkey PRIMARY KEY (id),
    CONSTRAINT fk_book_author_deferred FOREIGN KEY (author_id) REFERENCES authors (id) DEFERRABLE INITIALLY DEFERRED
);

--
-- Name: departments; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS departments (
    id SERIAL,
    name varchar(255) NOT NULL,
    CONSTRAINT departments_pkey PRIMARY KEY (id)
);

--
-- Name: employees; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS employees (
    id SERIAL,
    name varchar(255) NOT NULL,
    department_id integer,
    manager_id integer,
    CONSTRAINT employees_pkey PRIMARY KEY (id),
    CONSTRAINT fk_employee_department FOREIGN KEY (department_id) REFERENCES departments (id) ON DELETE CASCADE,
    CONSTRAINT fk_employee_manager FOREIGN KEY (manager_id) REFERENCES employees (id) ON DELETE SET NULL
);

--
-- Name: projects; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS projects (
    project_id integer,
    phase_id integer,
    name varchar(255) NOT NULL,
    CONSTRAINT projects_pkey PRIMARY KEY (project_id, phase_id)
);

--
-- Name: some_table; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS some_table (
    id SERIAL,
    name varchar(255) NOT NULL,
    CONSTRAINT some_table_pkey PRIMARY KEY (id)
);

--
-- Name: some_other_table; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS some_other_table (
    id SERIAL,
    some_table_id integer NOT NULL,
    CONSTRAINT some_other_table_pkey PRIMARY KEY (id),
    CONSTRAINT fk_rails_8bfc3d9c01 FOREIGN KEY (some_table_id) REFERENCES some_table (id)
);

--
-- Name: tasks; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS tasks (
    id SERIAL,
    project_id integer NOT NULL,
    phase_id integer NOT NULL,
    description text,
    CONSTRAINT tasks_pkey PRIMARY KEY (id),
    CONSTRAINT fk_task_project_phase FOREIGN KEY (project_id, phase_id) REFERENCES projects (project_id, phase_id)
);

