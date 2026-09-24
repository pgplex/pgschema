CREATE TABLE public.orders (
    id integer PRIMARY KEY,
    amount integer
);

-- A legacy row the new constraint must not be validated against
INSERT INTO public.orders (id, amount) VALUES (1, -5);
