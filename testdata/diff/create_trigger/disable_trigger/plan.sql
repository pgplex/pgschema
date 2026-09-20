ALTER TABLE employees ENABLE ALWAYS TRIGGER employees_always_trigger;

ALTER TABLE employees DISABLE TRIGGER employees_last_modified_trigger;

ALTER TABLE employees ENABLE REPLICA TRIGGER employees_replica_trigger;

ALTER TABLE employees ENABLE TRIGGER employees_reset_trigger;
