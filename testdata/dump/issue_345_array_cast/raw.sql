--
-- Test case for GitHub issue #345: pgschema dump strips type name from array literal casts
--
-- This reproduces the bug where explicit array type casts like ::text[] are
-- stripped, leaving invalid SQL like '{nested,key}'[]
--

CREATE TABLE repro (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    data jsonb DEFAULT '{}'
);

CREATE POLICY p ON repro USING ((data #>> '{nested,key}'::text[]) = 'x');
