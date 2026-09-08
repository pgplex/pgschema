ALTER TABLE users ADD CONSTRAINT users_email_not_null NOT NULL email NOT VALID;

ALTER TABLE users VALIDATE CONSTRAINT users_email_not_null;

ALTER TABLE users VALIDATE CONSTRAINT users_phone_not_null;
