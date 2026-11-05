CREATE FUNCTION is_valid_code(code text) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT code ~ '^[A-Z]{3}$' $$;

-- The parent's CHECK needs the new function, which puts it in a later
-- creation batch; the child must follow it there although the inherited
-- CHECK is not part of the child's own definition.
CREATE TABLE a_parent (
    code text NOT NULL,
    CONSTRAINT a_parent_code_check CHECK (is_valid_code(code))
);

CREATE TABLE b_child (
    detail text
) INHERITS (a_parent);
