ALTER FUNCTION expand_order(integer) ROWS 1;

CREATE OR REPLACE FUNCTION order_items(
    order_id integer
)
RETURNS SETOF integer
LANGUAGE sql
VOLATILE
ROWS 10
AS $$
    SELECT order_id + 1;
$$;

ALTER FUNCTION order_lines(integer) ROWS 1000;

CREATE OR REPLACE FUNCTION process_order(
    order_id integer,
    discount_percent numeric DEFAULT 0,
    status order_status DEFAULT 'pending'::order_status,
    priority utils.priority_level DEFAULT 'medium'::utils.priority_level
)
RETURNS numeric
LANGUAGE plpgsql
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
