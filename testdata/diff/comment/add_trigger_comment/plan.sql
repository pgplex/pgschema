COMMENT ON TRIGGER employees_last_modified_trigger ON employees IS 'Updates last_modified timestamp to current time on every row update';

COMMENT ON TRIGGER trg_employee_emails_insert ON employee_emails IS 'Handles inserts into the employee_emails view';
