--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.7.3


--
-- Name: employees; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS employees (
    id SERIAL,
    name varchar(100) NOT NULL,
    email varchar(100),
    CONSTRAINT employees_pkey PRIMARY KEY (id)
);

--
-- Name: employee_emails; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW employee_emails WITH (security_invoker = true) AS
 SELECT id,
    email
   FROM employees;

--
-- Name: employee_names; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW employee_names WITH (security_invoker = true) AS
 SELECT id,
    name
   FROM employees;

--
-- Name: employee_secure; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW employee_secure WITH (security_barrier = true) AS
 SELECT id,
    name
   FROM employees
  WHERE id > 0;

