SET check_function_bodies = false;

CREATE TABLE public.employees (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    email character varying(100)
);

CREATE SEQUENCE public.employees_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.employees_id_seq OWNED BY public.employees.id;

ALTER TABLE ONLY public.employees ALTER COLUMN id SET DEFAULT nextval('public.employees_id_seq'::regclass);

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_pkey PRIMARY KEY (id);

CREATE VIEW public.employee_names AS
 SELECT id,
    name
   FROM public.employees;

ALTER VIEW public.employee_names SET (security_invoker='true');

CREATE VIEW public.employee_emails AS
 SELECT id,
    email
   FROM public.employees;

ALTER VIEW public.employee_emails SET (security_invoker='true');

CREATE VIEW public.employee_secure AS
 SELECT id,
    name
   FROM public.employees
  WHERE (id > 0);

ALTER VIEW public.employee_secure SET (security_barrier='true');
