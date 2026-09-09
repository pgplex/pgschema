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
    CONSTRAINT employees_pkey PRIMARY KEY (id),
    CONSTRAINT employees_department_id_fkey FOREIGN KEY (department_id) REFERENCES public.departments(id)
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
    CONSTRAINT projects_pkey PRIMARY KEY (id),
    CONSTRAINT projects_tenant_id_org_id_fkey FOREIGN KEY (tenant_id, org_id) REFERENCES public.organizations(tenant_id, org_id)
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
    CONSTRAINT books_pkey PRIMARY KEY (id),
    CONSTRAINT books_author_id_fkey FOREIGN KEY (author_id) REFERENCES public.authors(id) ON DELETE CASCADE
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
    CONSTRAINT products_pkey PRIMARY KEY (id),
    CONSTRAINT products_category_code_fkey FOREIGN KEY (category_code) REFERENCES public.categories(code) ON UPDATE CASCADE
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
    CONSTRAINT teams_pkey PRIMARY KEY (id),
    CONSTRAINT teams_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES public.managers(id) ON DELETE SET NULL
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
    CONSTRAINT user_profiles_pkey PRIMARY KEY (user_id),
    CONSTRAINT user_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) DEFERRABLE INITIALLY DEFERRED
);

-- Self-referencing FK case
CREATE TABLE public.nodes (
    id integer NOT NULL,
    name text NOT NULL,
    parent_id integer,
    CONSTRAINT nodes_pkey PRIMARY KEY (id),
    CONSTRAINT nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.nodes(id)
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
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id),
    CONSTRAINT orders_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE,
    CONSTRAINT orders_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES public.managers(id) ON DELETE SET NULL
);

-- Composite FK with ON DELETE SET NULL / SET DEFAULT column list (PG15+, issue #589)
CREATE TABLE public.members (
    id integer NOT NULL,
    org_id integer NOT NULL,
    CONSTRAINT members_pkey PRIMARY KEY (id),
    CONSTRAINT members_org_id_id_key UNIQUE (org_id, id)
);

CREATE TABLE public.audit_log (
    id integer NOT NULL,
    org_id integer NOT NULL,
    actor_member_id integer,
    CONSTRAINT audit_log_pkey PRIMARY KEY (id),
    CONSTRAINT audit_log_org_id_actor_member_id_fkey FOREIGN KEY (org_id, actor_member_id) REFERENCES public.members(org_id, id) ON DELETE SET NULL (actor_member_id)
);

CREATE TABLE public.tasks (
    id integer NOT NULL,
    org_id integer NOT NULL,
    owner_member_id integer DEFAULT 0,
    CONSTRAINT tasks_pkey PRIMARY KEY (id),
    CONSTRAINT tasks_org_id_owner_member_id_fkey FOREIGN KEY (org_id, owner_member_id) REFERENCES public.members(org_id, id) ON DELETE SET DEFAULT (owner_member_id)
);

-- Existing composite FK that lacks the column list: the desired state adds it,
-- so the constraint must be recreated (the column list must be compared).
CREATE TABLE public.notes (
    id integer NOT NULL,
    org_id integer NOT NULL,
    author_member_id integer,
    CONSTRAINT notes_pkey PRIMARY KEY (id),
    CONSTRAINT notes_org_id_author_member_id_fkey FOREIGN KEY (org_id, author_member_id) REFERENCES public.members(org_id, id) ON DELETE SET NULL (author_member_id)
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
    CONSTRAINT price_adjustments_pkey PRIMARY KEY (id),
    CONSTRAINT price_adjustments_product_fkey FOREIGN KEY (product_id, PERIOD adjustment_period) REFERENCES public.price_history (product_id, PERIOD valid_period)
);
