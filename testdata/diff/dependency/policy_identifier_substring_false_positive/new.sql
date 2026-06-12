CREATE TABLE orders (
    id int NOT NULL,
    user_name text NOT NULL
);

ALTER TABLE orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY orders_current_user_scope ON orders
    FOR SELECT
    TO PUBLIC
    USING (user_name = CURRENT_USER);

CREATE TABLE "user" (
    id int NOT NULL
);
