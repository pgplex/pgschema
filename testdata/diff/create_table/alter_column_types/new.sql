CREATE TYPE public.action_type AS ENUM ('pending', 'approved', 'rejected');

CREATE COLLATION public.my_coll FROM "C";

CREATE TABLE public.user_pending_permissions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    permission text NOT NULL,
    object_ids_ints bigint[],
    action public.action_type,
    status public.action_type DEFAULT 'pending',
    tags public.action_type[],
    amount numeric(20,6) NOT NULL DEFAULT 0,
    arfcn_dl integer DEFAULT 0,
    priority integer,
    sort_key text COLLATE "C",
    label varchar(50),
    code text COLLATE "POSIX",
    region text COLLATE public.my_coll,
    zone text COLLATE public.my_coll
);