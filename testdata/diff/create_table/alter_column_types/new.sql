CREATE TYPE public.action_type AS ENUM ('pending', 'approved', 'rejected');

CREATE TABLE public.user_pending_permissions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    permission text NOT NULL,
    object_ids_ints bigint[],
    action public.action_type,
    status public.action_type DEFAULT 'pending',
    tags public.action_type[],
    amount numeric(20,6) NOT NULL DEFAULT 0
);