ALTER TABLE events ADD COLUMN priority integer DEFAULT 1;

COMMENT ON COLUMN alerts.priority IS 'Paging priority for this alert';
