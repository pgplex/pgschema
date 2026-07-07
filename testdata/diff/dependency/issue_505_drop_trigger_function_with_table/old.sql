CREATE TABLE events (
    id serial PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp DEFAULT CURRENT_TIMESTAMP
);

CREATE FUNCTION notify_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM pg_notify('events', NEW.id::text);
    RETURN NEW;
END;
$$;

CREATE TRIGGER events_notify
    AFTER INSERT ON events
    FOR EACH ROW
    EXECUTE FUNCTION notify_event();
