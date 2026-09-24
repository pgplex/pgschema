CREATE TABLE public.orders (
    id integer PRIMARY KEY,
    amount integer
);

-- Enforced for new rows only; existing rows stay unchecked
ALTER TABLE public.orders ADD CONSTRAINT orders_amount_check CHECK (amount >= 0) NOT VALID;
