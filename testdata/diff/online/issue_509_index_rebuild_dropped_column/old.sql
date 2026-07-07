CREATE TABLE public.notifications (
    id integer NOT NULL,
    tenant_id integer NOT NULL,
    user_id integer,
    valid_period text
);

CREATE INDEX idx_notifications_tenant_user ON public.notifications (tenant_id, user_id) WHERE valid_period IS NOT NULL;
