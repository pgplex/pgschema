CREATE TABLE public.payout (
    id varchar(20),
    partner_id varchar(20),
    CONSTRAINT payout_id_check CHECK (id IS NULL OR (id LIKE 'p-%' AND partner_id IS NOT NULL))
);
