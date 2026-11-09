CREATE TYPE order_status AS ENUM ('pending', 'processing', 'completed', 'cancelled');

CREATE FUNCTION process_order(
    order_id integer,
    discount_percent numeric DEFAULT 0,
    status order_status DEFAULT 'pending',
    priority utils.priority_level DEFAULT 'medium'
)
RETURNS numeric
LANGUAGE plpgsql
SECURITY DEFINER
VOLATILE
AS $$
DECLARE
    total numeric;
BEGIN
    SELECT amount INTO total FROM orders WHERE id = order_id;
    RETURN total - (total * discount_percent / 100);
END;
$$;

-- Set-returning function with the default ROWS estimate (1000); issue #702
CREATE FUNCTION expand_order(order_id integer)
RETURNS TABLE (line_id integer)
LANGUAGE sql
AS $$
    SELECT order_id;
$$;

-- Set-returning function with a custom estimate that goes back to the default; issue #702
CREATE FUNCTION order_lines(order_id integer)
RETURNS SETOF integer
LANGUAGE sql
ROWS 5
AS $$
    SELECT order_id;
$$;

-- Set-returning function whose body and estimate both change; issue #702
CREATE FUNCTION order_items(order_id integer)
RETURNS SETOF integer
LANGUAGE sql
AS $$
    SELECT order_id;
$$;
