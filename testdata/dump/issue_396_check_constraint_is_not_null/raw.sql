--
-- Test case for GitHub issue #396: Table-level CHECK constraints omitted from schema dump
--
-- CHECK constraints containing IS NOT NULL in complex expressions
-- are silently dropped because the inspector filters out any constraint
-- with "IS NOT NULL" in its expression, not just simple NOT NULL constraints.
--

CREATE TABLE test_table (
    id int PRIMARY KEY,
    status text NOT NULL,
    reason text,
    actor_id uuid,
    CONSTRAINT test_table_status_check CHECK (
        (status = 'active')
        OR (status = 'cancelled' AND reason IS NOT NULL)
        OR (status = 'revoked' AND actor_id IS NOT NULL)
    )
);
