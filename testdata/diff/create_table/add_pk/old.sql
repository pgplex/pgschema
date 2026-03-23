-- Basic single column case
CREATE TABLE public.users (
    username text NOT NULL,
    email text
);

-- Composite PK case
CREATE TABLE public.user_permissions (
    user_id integer NOT NULL,
    resource_id integer NOT NULL,
    permission_type text NOT NULL,
    granted_at timestamp with time zone
);

-- Identity column case
CREATE TABLE public.products (
    name text NOT NULL,
    price numeric(10,2)
);

-- Serial column case
CREATE TABLE public.orders (
    customer_id integer NOT NULL,
    order_date date DEFAULT CURRENT_DATE
);

-- UUID column case
CREATE TABLE public.sessions (
    user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);

-- Text column case
CREATE TABLE public.categories (
    name text NOT NULL,
    description text
);

-- Temporal PK case (PG18+)
CREATE TABLE public.reservations (
    id integer NOT NULL,
    valid_period tsrange NOT NULL,
    description text
);
