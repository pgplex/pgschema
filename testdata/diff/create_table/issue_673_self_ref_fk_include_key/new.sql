CREATE TABLE public.node (
    payload text,
    id integer NOT NULL,
    parent_id integer,
    CONSTRAINT node_pkey PRIMARY KEY (id) INCLUDE (payload),
    CONSTRAINT node_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.node (id)
);
