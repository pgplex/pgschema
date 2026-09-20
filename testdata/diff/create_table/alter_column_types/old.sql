CREATE COLLATION public.my_coll FROM "C";

CREATE TABLE public.user_pending_permissions (
    id integer NOT NULL,
    user_id integer NOT NULL,
    permission text NOT NULL,
    object_ids_ints integer[],
    action text,
    status text DEFAULT 'pending',
    tags text[],
    amount numeric(18,6) NOT NULL DEFAULT 0,
    arfcn_dl text DEFAULT 'unknown',
    priority text,
    sort_key text,
    label varchar(50) COLLATE "C",
    code text COLLATE "C",
    region text COLLATE public.my_coll,
    zone text
);