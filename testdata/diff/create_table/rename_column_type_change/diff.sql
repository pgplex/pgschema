ALTER TABLE orders DROP COLUMN status;

ALTER TABLE orders ADD COLUMN order_status integer NOT NULL;
