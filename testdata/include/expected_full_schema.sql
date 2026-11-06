-- Main schema file demonstrating \i include functionality
-- This represents a modular approach to organizing database schema
-- Includes ALL supported PostgreSQL database objects

-- Include custom types folder first (dependencies for tables)
--
-- Name: address; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE address AS (street text, city text);
--
-- Name: order_status; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE order_status AS ENUM (
    'pending',
    'completed'
);
--
-- Name: price_range; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE price_range AS RANGE (
    subtype = numeric
);
--
-- Name: user_status; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE user_status AS ENUM (
    'active',
    'inactive'
);

-- Include domain types (constrained base types)
--
-- Name: email_address; Type: DOMAIN; Schema: -; Owner: -
--

CREATE DOMAIN email_address AS text
  CONSTRAINT email_address_check CHECK (VALUE ~~ '%@%');
--
-- Name: positive_integer; Type: DOMAIN; Schema: -; Owner: -
--

CREATE DOMAIN positive_integer AS integer
  CONSTRAINT positive_integer_check CHECK (VALUE > 0);

-- Include sequences (may be used by tables)  
--
-- Name: global_id_seq; Type: SEQUENCE; Schema: -; Owner: -
--

CREATE SEQUENCE IF NOT EXISTS global_id_seq;
--
-- Name: order_number_seq; Type: SEQUENCE; Schema: -; Owner: -
--

CREATE SEQUENCE IF NOT EXISTS order_number_seq;

-- Include trigger function (needed by users table trigger)
--
-- Name: update_timestamp(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS trigger
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- Include core tables (with their constraints, indexes, and policies)
--
-- Name: users; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS users (
    id integer,
    email text NOT NULL,
    name text NOT NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id),
    CONSTRAINT users_email_check CHECK (email LIKE '%@%')
);

COMMENT ON TABLE users IS 'User accounts';

COMMENT ON COLUMN users.email IS 'User email address';

--
-- Name: idx_users_email; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);

--
-- Name: idx_users_name; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_users_name ON users (name);

--
-- Name: users; Type: RLS; Schema: -; Owner: -
--

ALTER TABLE users ENABLE ROW LEVEL SECURITY;

--
-- Name: users_policy; Type: POLICY; Schema: -; Owner: -
--

CREATE POLICY users_policy ON users TO PUBLIC USING (true);

--
-- Name: users_update_trigger; Type: TRIGGER; Schema: -; Owner: -
--

CREATE OR REPLACE TRIGGER users_update_trigger
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();
--
-- Name: orders; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    user_id integer NOT NULL,
    status text DEFAULT 'pending' NOT NULL,
    amount numeric(10,2) DEFAULT 0.00,
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT orders_status_check CHECK (status IN ('pending'::text, 'completed'::text))
);

COMMENT ON TABLE orders IS 'Customer orders';

COMMENT ON COLUMN orders.user_id IS 'Reference to user';

--
-- Name: idx_orders_status; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);

--
-- Name: idx_orders_user_id; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);

--
-- Name: orders; Type: RLS; Schema: -; Owner: -
--

ALTER TABLE orders ENABLE ROW LEVEL SECURITY;

--
-- Name: orders_policy; Type: POLICY; Schema: -; Owner: -
--

CREATE POLICY orders_policy ON orders TO PUBLIC USING (user_id = 1);

-- Include other functions (after tables that they reference)
--
-- Name: get_user_count(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_user_count()
RETURNS integer
LANGUAGE sql
VOLATILE
AS $$
    SELECT COUNT(*) FROM users;
$$;
--
-- Name: get_order_count(integer); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_order_count(
    user_id_param integer
)
RETURNS integer
LANGUAGE sql
VOLATILE
AS $$
    SELECT COUNT(*) FROM orders WHERE user_id = user_id_param;
$$;

-- Include procedures folder
--
-- Name: cleanup_orders(); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE cleanup_orders()
LANGUAGE sql
AS $$
    DELETE FROM orders WHERE status = 'completed';
$$;
--
-- Name: update_status(integer, text); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE update_status(
    IN user_id_param integer,
    IN new_status text
)
LANGUAGE sql
AS $$
    UPDATE orders SET status = new_status WHERE user_id = user_id_param;
$$;

-- Include views (depend on tables and functions)
--
-- Name: user_summary; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW user_summary AS
 SELECT u.id,
    u.name,
    count(o.id) AS order_count
   FROM users u
     JOIN orders o ON u.id = o.user_id
  GROUP BY u.id, u.name;

COMMENT ON VIEW user_summary IS 'User order summary';
--
-- Name: order_details; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW order_details AS
 SELECT o.id,
    o.status,
    u.name AS user_name
   FROM orders o
     JOIN users u ON o.user_id = u.id;

COMMENT ON VIEW order_details IS 'Order details with user info';

-- Include materialized views (depend on tables)
--
-- Name: user_order_summary; Type: MATERIALIZED VIEW; Schema: -; Owner: -
--

CREATE MATERIALIZED VIEW IF NOT EXISTS user_order_summary AS
 SELECT u.id AS user_id,
    u.name,
    u.email,
    count(o.id) AS total_orders,
    sum(o.amount) AS total_amount,
    max(o.amount) AS max_order_amount,
    avg(o.amount) AS avg_order_amount
   FROM users u
     LEFT JOIN orders o ON u.id = o.user_id
  GROUP BY u.id, u.name, u.email;

COMMENT ON MATERIALIZED VIEW user_order_summary IS 'Aggregated user order statistics';

--
-- Name: idx_user_order_summary_total_amount; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_user_order_summary_total_amount ON user_order_summary (total_amount DESC);

--
-- Name: idx_user_order_summary_user_id; Type: INDEX; Schema: -; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_order_summary_user_id ON user_order_summary (user_id);
--
-- Name: monthly_stats; Type: MATERIALIZED VIEW; Schema: -; Owner: -
--

CREATE MATERIALIZED VIEW IF NOT EXISTS monthly_stats AS
 SELECT date_trunc('month'::text, now()) AS month,
    count(user_id) AS unique_customers,
    count(id) AS total_orders,
    sum(amount) AS total_revenue,
    avg(amount) AS avg_order_value,
    count(
        CASE
            WHEN status = 'completed'::text THEN 1
            ELSE NULL::integer
        END) AS completed_orders,
    count(
        CASE
            WHEN status = 'pending'::text THEN 1
            ELSE NULL::integer
        END) AS pending_orders
   FROM orders o
  GROUP BY (date_trunc('month'::text, now()));

COMMENT ON MATERIALIZED VIEW monthly_stats IS 'Monthly order statistics';

--
-- Name: idx_monthly_stats_month; Type: INDEX; Schema: -; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_monthly_stats_month ON monthly_stats (month);

--
-- Name: idx_monthly_stats_revenue; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_monthly_stats_revenue ON monthly_stats (total_revenue DESC);
