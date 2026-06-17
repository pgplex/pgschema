CREATE TABLE public.orders (
    id serial PRIMARY KEY,
    user_id integer NOT NULL,
    status varchar(50),
    created_at timestamp DEFAULT CURRENT_TIMESTAMP
);
