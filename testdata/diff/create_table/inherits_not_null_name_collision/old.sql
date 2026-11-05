CREATE TABLE contacts (
    email text,
    CONSTRAINT email_not_null CHECK (email <> '')
);

CREATE TABLE users (
    nickname text
) INHERITS (contacts);
