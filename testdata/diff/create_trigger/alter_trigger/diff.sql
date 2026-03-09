CREATE OR REPLACE TRIGGER employees_last_modified_trigger
    BEFORE INSERT OR UPDATE OF salary ON employees
    FOR EACH ROW
    WHEN (((NEW.salary IS NOT NULL)))
    EXECUTE FUNCTION update_last_modified();
