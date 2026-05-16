CREATE TABLE public.reservations (
    id uuid,
    resource_id uuid NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    CONSTRAINT reservations_pkey PRIMARY KEY (id),
    CONSTRAINT valid_period CHECK (end_date >= start_date),
    CONSTRAINT no_overlap EXCLUDE USING gist (resource_id WITH =, daterange(start_date, end_date, '[]'::text) WITH &&)
);
