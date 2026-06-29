CREATE SEQUENCE public.user_id_seq;

COMMENT ON SEQUENCE public.user_id_seq IS 'Primary key sequence for the users table';

CREATE SEQUENCE public.order_id_seq AS integer START WITH 1000;

COMMENT ON SEQUENCE public.order_id_seq IS 'Primary key sequence for the orders table, starts at 1000';
