ALTER TABLE notifications DROP COLUMN valid_period;

DROP INDEX IF EXISTS idx_notifications_tenant_user;

CREATE INDEX IF NOT EXISTS idx_notifications_tenant_user ON notifications (tenant_id, user_id);
