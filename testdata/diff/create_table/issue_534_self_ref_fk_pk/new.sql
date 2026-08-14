CREATE TABLE categories (
    id bigint PRIMARY KEY,
    name text NOT NULL,
    parent_id bigint,
    CONSTRAINT fk_parent
        FOREIGN KEY (parent_id) REFERENCES categories (id)
);
