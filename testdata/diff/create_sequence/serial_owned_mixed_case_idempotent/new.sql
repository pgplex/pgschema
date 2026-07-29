CREATE TABLE orders (
    "orderId"  SERIAL PRIMARY KEY,
    amount     numeric(10,2) NOT NULL
);

CREATE TABLE users (
    "userId"   SERIAL PRIMARY KEY,
    email      text NOT NULL
);
