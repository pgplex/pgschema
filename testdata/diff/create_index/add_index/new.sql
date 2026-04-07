-- Create a new table with a simple index
CREATE TABLE public.users (
    id INTEGER PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(100)
);

CREATE INDEX idx_users_name ON public.users (name);
CREATE INDEX idx_users_email ON public.users (email varchar_pattern_ops);
CREATE INDEX idx_users_id ON public.users (id);
-- Test index name with dots (issue #196)
CREATE INDEX "public.idx_users" ON public.users (email, name);
-- Test NULLS NOT DISTINCT (issue #355)
CREATE UNIQUE INDEX idx_users_email_unique ON public.users (email) NULLS NOT DISTINCT;
-- Test INCLUDE columns (issue #385)
CREATE INDEX idx_users_email_include ON public.users (email) INCLUDE (name);
