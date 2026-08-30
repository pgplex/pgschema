--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.5.1


--
-- Name: audit; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS audit (
    id SERIAL,
    operation text NOT NULL,
    query text,
    user_name text NOT NULL,
    changed_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT audit_pkey PRIMARY KEY (id)
);

--
-- Name: idx_audit_changed_at; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_changed_at ON audit (changed_at);

--
-- Name: idx_audit_operation; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_operation ON audit (operation);

--
-- Name: idx_audit_username; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_username ON audit (user_name);

--
-- Name: audit; Type: RLS; Schema: -; Owner: -
--

ALTER TABLE audit ENABLE ROW LEVEL SECURITY;

--
-- Name: audit_insert_system; Type: POLICY; Schema: -; Owner: -
--

CREATE POLICY audit_insert_system ON audit FOR INSERT TO PUBLIC WITH CHECK (true);

--
-- Name: audit_user_isolation; Type: POLICY; Schema: -; Owner: -
--

CREATE POLICY audit_user_isolation ON audit TO PUBLIC USING (user_name = CURRENT_USER);

--
-- Name: department; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS department (
    dept_no text,
    dept_name text NOT NULL,
    CONSTRAINT department_pkey PRIMARY KEY (dept_no),
    CONSTRAINT department_dept_name_key UNIQUE (dept_name)
);

--
-- Name: employee; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS employee (
    emp_no SERIAL,
    birth_date date NOT NULL,
    first_name text NOT NULL,
    last_name text NOT NULL,
    gender text NOT NULL,
    hire_date date NOT NULL,
    CONSTRAINT employee_pkey PRIMARY KEY (emp_no),
    CONSTRAINT employee_gender_check CHECK (gender IN ('M'::text, 'F'::text))
);

--
-- Name: idx_employee_hire_date; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_employee_hire_date ON employee (hire_date);

--
-- Name: dept_emp; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS dept_emp (
    emp_no integer,
    dept_no text,
    from_date date NOT NULL,
    to_date date NOT NULL,
    CONSTRAINT dept_emp_pkey PRIMARY KEY (emp_no, dept_no),
    CONSTRAINT dept_emp_dept_no_fkey FOREIGN KEY (dept_no) REFERENCES department (dept_no) ON DELETE CASCADE,
    CONSTRAINT dept_emp_emp_no_fkey FOREIGN KEY (emp_no) REFERENCES employee (emp_no) ON DELETE CASCADE
);

--
-- Name: dept_manager; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS dept_manager (
    emp_no integer,
    dept_no text,
    from_date date NOT NULL,
    to_date date NOT NULL,
    CONSTRAINT dept_manager_pkey PRIMARY KEY (emp_no, dept_no),
    CONSTRAINT dept_manager_dept_no_fkey FOREIGN KEY (dept_no) REFERENCES department (dept_no) ON DELETE CASCADE,
    CONSTRAINT dept_manager_emp_no_fkey FOREIGN KEY (emp_no) REFERENCES employee (emp_no) ON DELETE CASCADE
);

--
-- Name: salary; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS salary (
    emp_no integer,
    amount integer NOT NULL,
    from_date date,
    to_date date NOT NULL,
    CONSTRAINT salary_pkey PRIMARY KEY (emp_no, from_date),
    CONSTRAINT salary_emp_no_fkey FOREIGN KEY (emp_no) REFERENCES employee (emp_no) ON DELETE CASCADE
);

--
-- Name: idx_salary_amount; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_salary_amount ON salary (amount);

--
-- Name: title; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS title (
    emp_no integer,
    title text,
    from_date date,
    to_date date,
    CONSTRAINT title_pkey PRIMARY KEY (emp_no, title, from_date),
    CONSTRAINT title_emp_no_fkey FOREIGN KEY (emp_no) REFERENCES employee (emp_no) ON DELETE CASCADE
);

--
-- Name: log_dml_operations(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION log_dml_operations()
RETURNS trigger
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    table_category TEXT;
    log_level TEXT;
BEGIN
    -- Get arguments passed from trigger (if any)
    -- TG_ARGV[0] is the first argument, TG_ARGV[1] is the second
    table_category := COALESCE(TG_ARGV[0], 'default');
    log_level := COALESCE(TG_ARGV[1], 'standard');

    IF (TG_OP = 'INSERT') THEN
        INSERT INTO audit (operation, query, user_name)
        VALUES (
            'INSERT [' || table_category || ':' || log_level || ']',
            current_query(),
            current_user
        );
        RETURN NEW;
    ELSIF (TG_OP = 'UPDATE') THEN
        INSERT INTO audit (operation, query, user_name)
        VALUES (
            'UPDATE [' || table_category || ':' || log_level || ']',
            current_query(),
            current_user
        );
        RETURN NEW;
    ELSIF (TG_OP = 'DELETE') THEN
        INSERT INTO audit (operation, query, user_name)
        VALUES (
            'DELETE [' || table_category || ':' || log_level || ']',
            current_query(),
            current_user
        );
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$;

--
-- Name: simple_salary_update(integer, integer); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE simple_salary_update(
    IN p_emp_no integer,
    IN p_amount integer
)
LANGUAGE plpgsql
AS $$
BEGIN
    -- Simple update of salary amount
    UPDATE salary 
    SET amount = p_amount 
    WHERE emp_no = p_emp_no 
    AND to_date = '9999-01-01';
    
    RAISE NOTICE 'Updated salary for employee % to $%', p_emp_no, p_amount;
END;
$$;

--
-- Name: salary_log_trigger; Type: TRIGGER; Schema: -; Owner: -
--

CREATE OR REPLACE TRIGGER salary_log_trigger
    AFTER UPDATE OR DELETE ON salary
    FOR EACH ROW
    EXECUTE FUNCTION log_dml_operations('payroll', 'high');

--
-- Name: dept_emp_latest_date; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW dept_emp_latest_date AS
 SELECT emp_no,
    max(from_date) AS from_date,
    max(to_date) AS to_date
   FROM dept_emp
  GROUP BY emp_no;

--
-- Name: current_dept_emp; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW current_dept_emp AS
 SELECT l.emp_no,
    d.dept_no,
    l.from_date,
    l.to_date
   FROM dept_emp d
     JOIN dept_emp_latest_date l ON d.emp_no = l.emp_no AND d.from_date = l.from_date AND l.to_date = d.to_date;

