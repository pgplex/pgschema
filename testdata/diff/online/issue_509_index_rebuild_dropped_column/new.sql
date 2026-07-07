CREATE TABLE public.notifications (
    id integer NOT NULL,
    tenant_id integer NOT NULL,
    user_id integer
);

CREATE INDEX idx_notifications_tenant_user ON public.notifications (tenant_id, user_id);
