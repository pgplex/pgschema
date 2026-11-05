-- users makes the inherited email NOT NULL. The online rewrite on
-- PostgreSQL 14-17 names its temporary CHECK <column>_not_null, which the
-- inherited CHECK copy already occupies, so it must pick another name.
CREATE TABLE contacts (
    email text,
    CONSTRAINT email_not_null CHECK (email <> '')
);

CREATE TABLE users (
    nickname text
) INHERITS (contacts);

ALTER TABLE users ALTER COLUMN email SET NOT NULL;
