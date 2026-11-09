CREATE TYPE order_status AS ENUM ('pending', 'processing', 'completed', 'cancelled');

CREATE FUNCTION process_order(
    order_id integer,
    discount_percent numeric DEFAULT 0,
    status order_status DEFAULT 'pending'::order_status,
    priority utils.priority_level DEFAULT 'medium'::utils.priority_level
)
RETURNS numeric
LANGUAGE plpgsql
SECURITY INVOKER
STABLE
AS $$
DECLARE
    base_price numeric;
    tax_rate numeric := 0.08;
BEGIN
    -- Different logic: calculate with tax instead of just discount
    -- Status and priority parameters are available but not used in this simplified version
    SELECT price INTO base_price FROM products WHERE id = order_id;
    RETURN base_price * (1 - discount_percent / 100) * (1 + tax_rate);
END;
$$;

-- Same body, but with a planner row estimate; issue #702
CREATE FUNCTION expand_order(order_id integer)
RETURNS TABLE (line_id integer)
LANGUAGE sql
ROWS 1
AS $$
    SELECT order_id;
$$;

-- Estimate dropped, back to the default of 1000; issue #702
CREATE FUNCTION order_lines(order_id integer)
RETURNS SETOF integer
LANGUAGE sql
AS $$
    SELECT order_id;
$$;

-- Body and estimate both change, so CREATE OR REPLACE carries ROWS; issue #702
CREATE FUNCTION order_items(order_id integer)
RETURNS SETOF integer
LANGUAGE sql
ROWS 10
AS $$
    SELECT order_id + 1;
$$;
