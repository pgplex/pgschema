-- Simple single-column FK case
CREATE TABLE public.departments (
    id integer NOT NULL,
    name text NOT NULL,
    CONSTRAINT departments_pkey PRIMARY KEY (id)
);

CREATE TABLE public.employees (
    id integer NOT NULL,
    name text NOT NULL,
    department_id integer NOT NULL,
    CONSTRAINT employees_pkey PRIMARY KEY (id)
);

-- Composite FK case
CREATE TABLE public.organizations (
    tenant_id integer NOT NULL,
    org_id integer NOT NULL,
    org_name text NOT NULL,
    CONSTRAINT organizations_pkey PRIMARY KEY (tenant_id, org_id)
);

CREATE TABLE public.projects (
    id integer NOT NULL,
    project_name text NOT NULL,
    tenant_id integer NOT NULL,
    org_id integer NOT NULL,
    CONSTRAINT projects_pkey PRIMARY KEY (id)
);

-- FK with ON DELETE CASCADE case
CREATE TABLE public.authors (
    id integer NOT NULL,
    name text NOT NULL,
    CONSTRAINT authors_pkey PRIMARY KEY (id)
);

CREATE TABLE public.books (
    id integer NOT NULL,
    title text NOT NULL,
    author_id integer NOT NULL,
    CONSTRAINT books_pkey PRIMARY KEY (id)
);

-- FK with ON UPDATE CASCADE case
CREATE TABLE public.categories (
    code text NOT NULL,
    name text NOT NULL,
    CONSTRAINT categories_pkey PRIMARY KEY (code)
);

CREATE TABLE public.products (
    id integer NOT NULL,
    name text NOT NULL,
    category_code text NOT NULL,
    CONSTRAINT products_pkey PRIMARY KEY (id)
);

-- FK with ON DELETE SET NULL case
CREATE TABLE public.managers (
    id integer NOT NULL,
    name text NOT NULL,
    CONSTRAINT managers_pkey PRIMARY KEY (id)
);

CREATE TABLE public.teams (
    id integer NOT NULL,
    name text NOT NULL,
    manager_id integer,
    CONSTRAINT teams_pkey PRIMARY KEY (id)
);

-- FK with DEFERRABLE case
CREATE TABLE public.users (
    id integer NOT NULL,
    username text NOT NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

CREATE TABLE public.user_profiles (
    user_id integer NOT NULL,
    bio text,
    CONSTRAINT user_profiles_pkey PRIMARY KEY (user_id)
);

-- Self-referencing FK case
CREATE TABLE public.nodes (
    id integer NOT NULL,
    name text NOT NULL,
    parent_id integer,
    CONSTRAINT nodes_pkey PRIMARY KEY (id)
);

-- Multiple FKs in a single table case
CREATE TABLE public.customers (
    id integer NOT NULL,
    name text NOT NULL,
    CONSTRAINT customers_pkey PRIMARY KEY (id)
);

CREATE TABLE public.orders (
    id integer NOT NULL,
    customer_id integer NOT NULL,
    product_id integer NOT NULL,
    manager_id integer,
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);

-- Temporal FK case (PG18+)
CREATE TABLE public.price_history (
    product_id integer NOT NULL,
    valid_period tsrange NOT NULL,
    price numeric(10,2) NOT NULL,
    CONSTRAINT price_history_pkey PRIMARY KEY (product_id, valid_period WITHOUT OVERLAPS)
);

CREATE TABLE public.price_adjustments (
    id integer NOT NULL,
    product_id integer NOT NULL,
    adjustment_period tsrange NOT NULL,
    adjustment_pct numeric(5,2) NOT NULL,
    CONSTRAINT price_adjustments_pkey PRIMARY KEY (id)
);
