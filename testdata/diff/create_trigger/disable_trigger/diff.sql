CREATE OR REPLACE TRIGGER employees_new_replica_trigger
    BEFORE UPDATE ON employees
    FOR EACH ROW
    EXECUTE FUNCTION update_last_modified();

ALTER TABLE employees ENABLE REPLICA TRIGGER employees_new_replica_trigger;

ALTER TABLE employees ENABLE ALWAYS TRIGGER employees_always_trigger;

ALTER TABLE employees DISABLE TRIGGER employees_last_modified_trigger;

CREATE OR REPLACE TRIGGER employees_recreate_trigger
    BEFORE INSERT OR UPDATE ON employees
    FOR EACH ROW
    EXECUTE FUNCTION update_last_modified();

ALTER TABLE employees ENABLE ALWAYS TRIGGER employees_recreate_trigger;

ALTER TABLE employees ENABLE REPLICA TRIGGER employees_replica_trigger;

ALTER TABLE employees ENABLE TRIGGER employees_reset_trigger;
