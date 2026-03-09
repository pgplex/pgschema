ALTER VIEW employee_emails SET (security_invoker=true);
ALTER VIEW employee_names SET (security_invoker=true);
ALTER VIEW employee_secure RESET (security_invoker);
