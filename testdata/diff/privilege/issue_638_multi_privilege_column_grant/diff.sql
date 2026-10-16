REVOKE INSERT (a, b), UPDATE (a, b) ON TABLE t FROM app_user;

GRANT INSERT (x), UPDATE (x) ON TABLE u TO app_user;

GRANT INSERT (a), UPDATE (a) ON TABLE t TO app_user;
