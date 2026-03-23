ALTER TABLE categories
ADD COLUMN code text CONSTRAINT categories_pkey PRIMARY KEY;

ALTER TABLE orders
ADD COLUMN id serial CONSTRAINT orders_pkey PRIMARY KEY;

ALTER TABLE products
ADD COLUMN id integer GENERATED ALWAYS AS IDENTITY CONSTRAINT products_pkey PRIMARY KEY;

ALTER TABLE reservations
ADD CONSTRAINT reservations_pkey PRIMARY KEY (id, valid_period WITHOUT OVERLAPS);

ALTER TABLE sessions
ADD COLUMN id uuid CONSTRAINT sessions_pkey PRIMARY KEY;

ALTER TABLE user_permissions
ADD CONSTRAINT user_permissions_pkey PRIMARY KEY (user_id, resource_id, permission_type);

ALTER TABLE users
ADD COLUMN id integer CONSTRAINT users_pkey PRIMARY KEY;
