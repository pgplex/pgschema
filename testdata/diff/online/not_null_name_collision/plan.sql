ALTER TABLE users ADD CONSTRAINT users_email_not_null1 NOT NULL email NOT VALID;

ALTER TABLE users VALIDATE CONSTRAINT users_email_not_null1;
