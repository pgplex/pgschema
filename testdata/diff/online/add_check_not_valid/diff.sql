ALTER TABLE orders
ADD CONSTRAINT orders_amount_check CHECK (amount >= 0) NOT VALID;
