CREATE TABLE public.node (
    payload text,
    parent_id integer,
    id integer NOT NULL,
    CONSTRAINT node_pkey PRIMARY KEY (id),
    CONSTRAINT node_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.node (id)
);
