-- Basic single column case
CREATE TABLE public.users (
    id integer,
    username text NOT NULL,
    email text,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

-- Composite PK case
CREATE TABLE public.user_permissions (
    user_id integer NOT NULL,
    resource_id integer NOT NULL,
    permission_type text NOT NULL,
    granted_at timestamp with time zone,
    CONSTRAINT user_permissions_pkey PRIMARY KEY (user_id, resource_id, permission_type)
);

-- Identity column case
CREATE TABLE public.products (
    id integer GENERATED ALWAYS AS IDENTITY,
    name text NOT NULL,
    price numeric(10,2),
    CONSTRAINT products_pkey PRIMARY KEY (id)
);

-- Serial column case
CREATE TABLE public.orders (
    id serial,
    customer_id integer NOT NULL,
    order_date date DEFAULT CURRENT_DATE,
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);

-- UUID column case
CREATE TABLE public.sessions (
    id uuid,
    user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT sessions_pkey PRIMARY KEY (id)
);

-- Text column case
CREATE TABLE public.categories (
    code text,
    name text NOT NULL,
    description text,
    CONSTRAINT categories_pkey PRIMARY KEY (code)
);

-- Temporal PK case (PG18+)
CREATE TABLE public.reservations (
    id integer NOT NULL,
    valid_period tsrange NOT NULL,
    description text,
    CONSTRAINT reservations_pkey PRIMARY KEY (id, valid_period WITHOUT OVERLAPS)
);
