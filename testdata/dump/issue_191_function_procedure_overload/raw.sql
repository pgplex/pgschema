--
-- Test case for GitHub issue #191: Overloaded functions and procedures not fully dumped
-- Also covers #368: dump emits wrong LANGUAGE for overloaded functions with mixed languages
--
-- #191: Only the last overloaded function/procedure was included in the dump output.
--       Functions and procedures were stored by name only, causing overloads with
--       different signatures to overwrite each other.
-- #368: When overloaded functions have different languages (e.g., sql vs plpgsql),
--       the dump may assign the wrong language due to a cross-join in the query
--       between information_schema.routines and pg_proc.
--

--
-- Function overloads: 3 versions of test_func with different signatures
--

-- Overload 1: Single integer parameter
CREATE OR REPLACE FUNCTION test_func(a integer)
RETURNS integer
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN a * 2;
END;
$$;

-- Overload 2: Two integer parameters (different count)
CREATE OR REPLACE FUNCTION test_func(a integer, b integer)
RETURNS integer
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN a + b;
END;
$$;

-- Overload 3: Single text parameter (different type)
CREATE OR REPLACE FUNCTION test_func(a text)
RETURNS text
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN 'Hello, ' || a;
END;
$$;

--
-- Procedure overloads: 3 versions of test_proc with different signatures
--

-- Overload 1: Single integer parameter
CREATE OR REPLACE PROCEDURE test_proc(a integer)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Integer: %', a;
END;
$$;

-- Overload 2: Two integer parameters (different count)
CREATE OR REPLACE PROCEDURE test_proc(a integer, b integer)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Sum: %', a + b;
END;
$$;

-- Overload 3: Single text parameter (different type)
CREATE OR REPLACE PROCEDURE test_proc(a text)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Text: %', a;
END;
$$;

--
-- Mixed-language function overloads (#368): same name, different languages
--

-- Overload 1: SQL language
CREATE OR REPLACE FUNCTION provide_tx(VARIADIC p_txs text[])
RETURNS void
LANGUAGE sql
AS $$
SELECT 1;
$$;

-- Overload 2: plpgsql language
CREATE OR REPLACE FUNCTION provide_tx(p_id uuid)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE '%', p_id;
END;
$$;
