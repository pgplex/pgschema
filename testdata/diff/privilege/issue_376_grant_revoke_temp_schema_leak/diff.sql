REVOKE EXECUTE ON FUNCTION f_test(p_items my_input[]) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION f_test(p_items my_input[]) TO appname_apiuser;
